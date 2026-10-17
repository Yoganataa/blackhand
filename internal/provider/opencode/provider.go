package opencode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
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
type ProxyOptions struct { URL string; NoProxy string; RelayURL string; Strict bool }

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
func(p *Provider)ProxyChatWithProxy(w http.ResponseWriter,r *http.Request,proxyURL string)ProxyResult {
	return p.ProxyChatWithOptions(w,r,ProxyOptions{URL:proxyURL})
}
func(p *Provider)ProxyChatWithOptions(w http.ResponseWriter,r *http.Request,options ProxyOptions)ProxyResult {
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
	targetURL,parseErr:=url.Parse(target)
	if parseErr!=nil { http.Error(w,"invalid upstream URL",502); result.Error="invalid upstream URL"; return result }
	relayURL:=strings.TrimSpace(options.RelayURL)
	proxyURL:=strings.TrimSpace(options.URL)
	if shouldBypassHost(targetURL.Hostname(),options.NoProxy) {
		relayURL=""
		proxyURL=""
	}
	requestURL:=target
	if relayURL!="" { requestURL=relayURL }
	makeRequest:=func()(*http.Request,error){
		req,err:=http.NewRequestWithContext(r.Context(),http.MethodPost,requestURL,bytes.NewReader(body))
		if err!=nil{return nil,err}
		req.ContentLength=int64(len(body))
		for _,header:=range []string{"Content-Type","Accept"}{if value:=r.Header.Get(header);value!=""{req.Header.Set(header,value)}}
		if p.cfg.UpstreamAPIKey!=""{req.Header.Set("Authorization","Bearer "+p.cfg.UpstreamAPIKey)}
		if relayURL!="" {
			req.Header.Set("x-relay-target",targetURL.Scheme+"://"+targetURL.Host)
			req.Header.Set("x-relay-path",targetURL.RequestURI())
		}
		return req,nil
	}
	client:=p.upstreamClient
	var transport *http.Transport
	if proxyURL!=""{
		parsed,proxyErr:=url.Parse(proxyURL)
		if proxyErr!=nil||(parsed.Scheme!="http"&&parsed.Scheme!="https")||parsed.Host==""{http.Error(w,"invalid outbound proxy URL",502);result.Error="invalid outbound proxy URL";return result}
		base,ok:=http.DefaultTransport.(*http.Transport);if !ok{http.Error(w,"invalid HTTP transport",502);return result}
		transport=base.Clone()
		transport.Proxy=func(req *http.Request)(*url.URL,error){
			if shouldBypassHost(req.URL.Hostname(),options.NoProxy){return nil,nil}
			return parsed,nil
		}
		defer transport.CloseIdleConnections()
		client=&http.Client{Transport:transport,Timeout:p.cfg.UpstreamTimeout}
	}
	req,err:=makeRequest()
	if err!=nil{http.Error(w,"invalid upstream request",502);result.Error="invalid upstream request";return result}
	resp,err:=client.Do(req)
	if err!=nil{
		if errors.Is(err,context.Canceled)||errors.Is(r.Context().Err(),context.Canceled){result.Error="request cancelled";return result}
		// 9Router falls back to direct networking when strictProxy is false.
		// The retry is safe here because no response body has been sent to the client.
		if client!=p.upstreamClient&&!options.Strict{
			directTarget:=target
			if relayURL!="" { directTarget=target }
			directReq,buildErr:=http.NewRequestWithContext(r.Context(),http.MethodPost,directTarget,bytes.NewReader(body))
			if buildErr==nil{
				directReq.ContentLength=int64(len(body))
				for _,header:=range []string{"Content-Type","Accept"}{if value:=r.Header.Get(header);value!=""{directReq.Header.Set(header,value)}}
				if p.cfg.UpstreamAPIKey!=""{directReq.Header.Set("Authorization","Bearer "+p.cfg.UpstreamAPIKey)}
				resp,err=p.upstreamClient.Do(directReq)
			}
		}
		if err!=nil{http.Error(w,"upstream request failed",502);result.Error="upstream request failed";return result}
	}
	defer resp.Body.Close()
	result.StatusCode=resp.StatusCode
	for _,header:=range []string{"Content-Type","Cache-Control","X-Request-Id","Retry-After"}{if value:=resp.Header.Get(header);value!=""{w.Header().Set(header,value)}}
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")),"text/event-stream"){
		w.WriteHeader(resp.StatusCode)
		flusher,canFlush:=w.(http.Flusher)
		reader:=bufio.NewReaderSize(resp.Body,32*1024)
		for{
			line,readErr:=reader.ReadBytes('\n')
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
func shouldBypassHost(host,noProxy string) bool {
	host=strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)),".")
	if host==""||strings.TrimSpace(noProxy)==""{return false}
	for _,entry:=range strings.FieldsFunc(noProxy,func(r rune)bool{return r==','||r==' '||r=='\t'||r=='\n'}){
		pattern:=strings.ToLower(strings.TrimSpace(entry))
		if pattern==""{continue}
		if pattern=="*"{return true}
		if strings.Contains(pattern,"/"){
			_,network,err:=net.ParseCIDR(pattern)
			if err==nil { if ip:=net.ParseIP(host);ip!=nil&&network.Contains(ip){return true} }
			continue
		}
		pattern=strings.TrimSuffix(pattern,".")
		if host==strings.TrimPrefix(pattern,".")||strings.HasSuffix(host,"."+strings.TrimPrefix(pattern,".")){return true}
	}
	return false
}

func parseSSEUsage(line []byte,usage map[string]any){
	raw:=bytes.TrimSpace(bytes.TrimPrefix(bytes.TrimSpace(line),[]byte("data:")))
	if len(raw)==0||bytes.Equal(raw,[]byte("[DONE]")){return}
	var event map[string]any
	if json.Unmarshal(raw,&event)!=nil{return}
	candidate,ok:=event["usage"].(map[string]any);if !ok{return}
	for key,value:=range candidate{usage[key]=value}
}
