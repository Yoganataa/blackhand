package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type proxyTestResult struct {
	OK bool `json:"ok"`
	Status int `json:"status"`
	StatusText string `json:"statusText"`
	Error string `json:"error,omitempty"`
	ElapsedMS int64 `json:"elapsedMs"`
	TestedAt string `json:"testedAt,omitempty"`
}

func testProxyPool(ctx context.Context,pool map[string]any)proxyTestResult{
	started:=time.Now()
	proxyURL,_:=pool["proxyUrl"].(string)
	kind,_:=pool["type"].(string)
	proxyURL=strings.TrimSpace(proxyURL)
	if proxyURL==""{return proxyTestResult{Status:400,StatusText:"Bad Request",Error:"proxyUrl is required"}}
	if kind=="vercel"||kind=="cloudflare"||kind=="deno"||kind=="netlify"{
		return testRelay(ctx,proxyURL,started)
	}
	parsed,err:=url.Parse(proxyURL)
	if err!=nil||(parsed.Scheme!="http"&&parsed.Scheme!="https")||parsed.Host==""{
		return proxyTestResult{Status:400,StatusText:"Bad Request",Error:"Invalid proxy URL: expected http:// or https:// URL"}
	}
	transport:=http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy=http.ProxyURL(parsed)
	client:=&http.Client{Transport:transport,Timeout:8*time.Second}
	defer transport.CloseIdleConnections()
	req,err:=http.NewRequestWithContext(ctx,http.MethodHead,"https://google.com/",nil)
	if err!=nil{return proxyTestResult{Status:400,StatusText:"Bad Request",Error:err.Error()}}
	req.Header.Set("User-Agent","9Router")
	res,err:=client.Do(req)
	if err!=nil{return proxyTestResult{Status:500,StatusText:"Proxy Error",Error:safeError(err),ElapsedMS:time.Since(started).Milliseconds()}}
	defer res.Body.Close()
	result:=proxyTestResult{OK:res.StatusCode>=200&&res.StatusCode<300,Status:res.StatusCode,StatusText:res.Status,ElapsedMS:time.Since(started).Milliseconds()}
	if !result.OK{result.Error=fmt.Sprintf("Proxy test failed with status %d",res.StatusCode)}
	return result
}

func testRelay(ctx context.Context,relayURL string,started time.Time)proxyTestResult{
	parsed,err:=url.Parse(relayURL)
	if err!=nil||(parsed.Scheme!="http"&&parsed.Scheme!="https")||parsed.Host==""{
		return proxyTestResult{Status:400,StatusText:"Bad Request",Error:"Invalid relay URL"}
	}
	req,err:=http.NewRequestWithContext(ctx,http.MethodGet,relayURL,nil)
	if err!=nil{return proxyTestResult{Status:400,StatusText:"Bad Request",Error:err.Error()}}
	req.Header.Set("x-relay-target","https://httpbin.org")
	req.Header.Set("x-relay-path","/get")
	client:=&http.Client{Timeout:10*time.Second}
	res,err:=client.Do(req)
	if err!=nil{return proxyTestResult{Status:500,StatusText:"Relay Error",Error:safeError(err),ElapsedMS:time.Since(started).Milliseconds()}}
	defer res.Body.Close()
	result:=proxyTestResult{OK:res.StatusCode>=200&&res.StatusCode<300,Status:res.StatusCode,StatusText:res.Status,ElapsedMS:time.Since(started).Milliseconds()}
	if !result.OK{result.Error=fmt.Sprintf("Relay test failed with status %d",res.StatusCode)}
	return result
}
func safeError(err error)string{
	value:=err.Error()
	if len(value)>500{value=value[:500]}
	// Avoid echoing proxy credentials that may be embedded in a URL.
	return value
}
