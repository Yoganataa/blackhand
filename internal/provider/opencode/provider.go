package opencode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Yoganataa/blackhand/internal/config"
)

type Provider struct {
	cfg config.Config
	discoveryClient *http.Client
	upstreamClient *http.Client
	mu sync.Mutex
	cached []Model
	cacheAt time.Time
}
type Model struct {
	ID string `json:"id"`
	Object string `json:"object,omitempty"`
	Created int64 `json:"created,omitempty"`
	OwnedBy string `json:"owned_by,omitempty"`
}
type modelResponse struct { Data []Model `json:"data"` }
type ProxyResult struct { StatusCode int; Usage map[string]any; Error string }

func New(cfg config.Config) *Provider {
	return &Provider{cfg:cfg,discoveryClient:&http.Client{Timeout:cfg.DiscoveryTimeout},upstreamClient:&http.Client{Timeout:cfg.UpstreamTimeout}}
}
func (p *Provider) Models(ctx context.Context)([]Model,error){
	p.mu.Lock()
	if len(p.cached)>0&&time.Since(p.cacheAt)<p.cfg.ModelCacheTTL{out:=append([]Model(nil),p.cached...);p.mu.Unlock();return prefixModels(out),nil}
	p.mu.Unlock()
	req,err:=http.NewRequestWithContext(ctx,http.MethodGet,p.cfg.ModelsURL,nil);if err!=nil{return nil,fmt.Errorf("build model discovery request: %w",err)}
	if p.cfg.UpstreamAPIKey!=""{req.Header.Set("Authorization","Bearer "+p.cfg.UpstreamAPIKey)}
	resp,err:=p.discoveryClient.Do(req);if err!=nil{return p.staleOrError(fmt.Errorf("model discovery: %w",err))}
	defer resp.Body.Close()
	if resp.StatusCode<200||resp.StatusCode>=300{_,_=io.Copy(io.Discard,io.LimitReader(resp.Body,4096));return p.staleOrError(fmt.Errorf("model discovery returned HTTP %d",resp.StatusCode))}
	var payload modelResponse
	if err:=json.NewDecoder(io.LimitReader(resp.Body,4<<20)).Decode(&payload);err!=nil{return p.staleOrError(fmt.Errorf("decode model discovery: %w",err))}
	for i:=range payload.Data{payload.Data[i].ID=strings.TrimPrefix(payload.Data[i].ID,"oc/");if payload.Data[i].Object==""{payload.Data[i].Object="model"};if payload.Data[i].OwnedBy==""{payload.Data[i].OwnedBy="opencode"}}
	p.mu.Lock();p.cached=append([]Model(nil),payload.Data...);p.cacheAt=time.Now();p.mu.Unlock()
	return prefixModels(payload.Data),nil
}
func(p *Provider)staleOrError(err error)([]Model,error){p.mu.Lock();defer p.mu.Unlock();if len(p.cached)>0{return prefixModels(p.cached),nil};return nil,err}
func prefixModels(models []Model)[]Model{out:=make([]Model,len(models));copy(out,models);for i:=range out{if !strings.HasPrefix(out[i].ID,"oc/"){out[i].ID="oc/"+out[i].ID}};return out}

// ProxyChat preserves incremental SSE delivery, forwards cancellation, and
// extracts usage without retaining the full generated response in memory.
func(p *Provider)ProxyChat(w http.ResponseWriter,r *http.Request){_ = p.ProxyChatWithProxy(w,r,"")}
func(p *Provider)ProxyChatWithProxy(w http.ResponseWriter,r *http.Request,proxyURL string)ProxyResult{
	result:=ProxyResult{StatusCode:502,Usage:map[string]any{}}
	body,err:=io.ReadAll(http.MaxBytesReader(w,r.Body,16<<20))
	if err!=nil{http.Error(w,"request body is too large or invalid",http.StatusRequestEntityTooLarge);result.Error="request body invalid";return result}
	var payload map[string]json.RawMessage
	if json.Unmarshal(body,&payload)==nil{
		var model string
		if raw,ok:=payload["model"];ok&&json.Unmarshal(raw,&model)==nil&&strings.HasPrefix(model,"oc/"){
			encoded,_:=json.Marshal(strings.TrimPrefix(model,"oc/"));payload["model"]=encoded
			if rewritten,e:=json.Marshal(payload);e==nil{body=rewritten}
		}
	}
	target:=strings.TrimRight(p.cfg.UpstreamBaseURL,"/")+"/chat/completions"
	req,err:=http.NewRequestWithContext(r.Context(),http.MethodPost,target,bytes.NewReader(body))
	if err!=nil{http.Error(w,"invalid upstream request",502);result.Error="invalid upstream request";return result}
	req.ContentLength=int64(len(body))
	for _,header:=range []string{"Content-Type","Accept"}{if value:=r.Header.Get(header);value!=""{req.Header.Set(header,value)}}
	if p.cfg.UpstreamAPIKey!=""{req.Header.Set("Authorization","Bearer "+p.cfg.UpstreamAPIKey)}
	client:=p.upstreamClient
	var transport *http.Transport
	if strings.TrimSpace(proxyURL)!=""{
		parsed,parseErr:=url.Parse(proxyURL)
		if parseErr!=nil||(parsed.Scheme!="http"&&parsed.Scheme!="https")||parsed.Host==""{http.Error(w,"invalid outbound proxy URL",502);result.Error="invalid outbound proxy URL";return result}
		base,ok:=http.DefaultTransport.(*http.Transport);if !ok{http.Error(w,"invalid HTTP transport",502);return result}
		transport=base.Clone();transport.Proxy=http.ProxyURL(parsed);defer transport.CloseIdleConnections()
		client=&http.Client{Transport:transport,Timeout:p.cfg.UpstreamTimeout}
	}
	resp,err:=client.Do(req)
	if err!=nil{
		if errors.Is(err,context.Canceled)||errors.Is(r.Context().Err(),context.Canceled){result.Error="request cancelled";return result}
		http.Error(w,"upstream request failed",502);result.Error="upstream request failed";return result
	}
	defer resp.Body.Close()
	result.StatusCode=resp.StatusCode
	for _,header:=range []string{"Content-Type","Cache-Control","X-Request-Id","Retry-After"}{if value:=resp.Header.Get(header);value!=""{w.Header().Set(header,value)}}
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")),"text/event-stream"){
		w.WriteHeader(resp.StatusCode)
		flusher,canFlush:=w.(http.Flusher)
		reader:=bufio.NewReaderSize(resp.Body,32*1024)
		for{
			line,readErr:=reader.ReadBytes('
')
			if len(line)>0{
				if bytes.HasPrefix(bytes.TrimSpace(line),[]byte("data:")){parseSSEUsage(line,result.Usage)}
				if _,writeErr:=w.Write(line);writeErr!=nil{return result}
				if canFlush{flusher.Flush()}
			}
			if readErr!=nil{if readErr!=io.EOF{result.Error="stream read failed"};return result}
		}
	}
	const maxResponseBytes=32<<20
	responseBody,err:=io.ReadAll(io.LimitReader(resp.Body,maxResponseBytes+1))
	if err!=nil{http.Error(w,"failed to read upstream response",502);result.Error="failed to read upstream response";return result}
	if len(responseBody)>maxResponseBytes{http.Error(w,"upstream response exceeded size limit",502);result.StatusCode=502;result.Error="upstream response exceeded size limit";return result}
	var decoded map[string]any
	if json.Unmarshal(responseBody,&decoded)==nil{if usage,ok:=decoded["usage"].(map[string]any);ok{result.Usage=usage}}
	w.WriteHeader(resp.StatusCode);_,_=w.Write(responseBody)
	return result
}
func parseSSEUsage(line []byte,usage map[string]any){
	raw:=bytes.TrimSpace(bytes.TrimPrefix(bytes.TrimSpace(line),[]byte("data:")))
	if len(raw)==0||bytes.Equal(raw,[]byte("[DONE]")){return}
	var event map[string]any
	if json.Unmarshal(raw,&event)!=nil{return}
	candidate,ok:=event["usage"].(map[string]any);if !ok{return}
	for key,value:=range candidate{usage[key]=value}
}
