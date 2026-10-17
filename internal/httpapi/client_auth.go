package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Yoganataa/blackhand/internal/storage"
)

func extractClientKey(r *http.Request) string {
	auth:=r.Header.Get("Authorization")
	if len(auth)>=7&&strings.EqualFold(auth[:7],"Bearer "){return strings.TrimSpace(auth[7:])}
	for _,header:=range []string{"x-api-key","x-goog-api-key"}{if value:=strings.TrimSpace(r.Header.Get(header));value!=""{return value}}
	return strings.TrimSpace(r.URL.Query().Get("key"))
}
func (a *API) withClientKey(next func(http.ResponseWriter,*http.Request,*storage.APIKey,string))http.HandlerFunc{
	return func(w http.ResponseWriter,r *http.Request){
		settings,err:=a.store.GetSettings(r.Context());if err!=nil{writeJSON(w,500,map[string]string{"error":"Could not load router settings"});return}
		require:=enabledSetting(settings["requireApiKey"],true)
		if raw,ok:=os.LookupEnv("REQUIRE_API_KEY");ok{require=strings.EqualFold(strings.TrimSpace(raw),"true")||strings.TrimSpace(raw)=="1"}
		presented:=extractClientKey(r)
		if presented==""&&require{writeJSON(w,401,map[string]any{"error":map[string]string{"message":"API key is required","type":"authentication_error"}});return}
		var key *storage.APIKey
		if presented!=""{
			key,err=a.store.GetAPIKeyByKey(r.Context(),presented)
			if err!=nil{writeJSON(w,500,map[string]string{"error":"API key validation failed"});return}
			if key==nil||!key.IsActive{writeJSON(w,401,map[string]any{"error":map[string]string{"message":"Invalid or inactive API key","type":"authentication_error"}});return}
		}
		next(w,r,key,presented)
	}
}
func (a *API) models(w http.ResponseWriter,r *http.Request){
	a.withClientKey(func(w http.ResponseWriter,r *http.Request,key *storage.APIKey,_ string){
		ctx,cancel:=context.WithTimeout(r.Context(),a.cfg.DiscoveryTimeout);defer cancel()
		models,err:=a.provider.Models(ctx)
		if err!=nil{a.logger.Warn("OpenCode model discovery failed","error",err);writeJSON(w,502,map[string]any{"error":map[string]string{"message":"Model discovery failed","type":"upstream_error"}});return}
		if key!=nil&&key.Access.Restricted{
			filtered:=models[:0];for _,model:=range models{if a.store.AllowedForModel(key,model.ID){filtered=append(filtered,model)}};models=filtered
		}
		writeJSON(w,200,map[string]any{"object":"list","data":models})
	})(w,r)
}
func (a *API) chat(w http.ResponseWriter,r *http.Request){
	a.withClientKey(func(w http.ResponseWriter,r *http.Request,key *storage.APIKey,presented string){
		r.Body=http.MaxBytesReader(w,r.Body,16<<20)
		raw,err:=io.ReadAll(r.Body);if err!=nil{writeJSON(w,413,map[string]string{"error":"Request body is too large"});return}
		var body map[string]any
		decoder:=json.NewDecoder(bytes.NewReader(raw));decoder.UseNumber()
		if err:=decoder.Decode(&body);err!=nil||body==nil{writeJSON(w,400,map[string]any{"error":map[string]any{"message":"Invalid JSON request body","type":"invalid_request_error"}});return}
		model,_:=body["model"].(string);if strings.TrimSpace(model)==""{writeJSON(w,400,map[string]any{"error":map[string]any{"message":"model is required","type":"invalid_request_error"}});return}
		if key!=nil&&!a.store.AllowedForModel(key,model){writeJSON(w,403,map[string]any{"error":map[string]any{"message":"This API key is not allowed to access the requested model","type":"permission_error"}});return}
		settings,err:=a.store.GetSettings(r.Context());if err!=nil{writeJSON(w,500,map[string]string{"error":"Could not load router settings"});return}
		saverStats:=a.tokenSaver.Process(r.Context(),body,settings,model)
		prepared,err:=json.Marshal(body);if err!=nil{writeJSON(w,400,map[string]string{"error":"Could not encode request"});return}
		r.Body=io.NopCloser(bytes.NewReader(prepared))
		proxyURL:=""
		if enabledSetting(settings["outboundProxyEnabled"],false){proxyURL,_=settings["outboundProxyUrl"].(string)}
		a.beginRequest(model);started:=time.Now()
		result:=a.provider.ProxyChatWithProxy(w,r,proxyURL)
		a.endRequest(model)
		if result.Error!=""{a.appendLog("OpenCode request ended: "+result.Error)}
		meta:=map[string]any{"tokenSaver":saverStats,"durationMs":time.Since(started).Milliseconds()}
		if saverStats.Headroom!=""&&saverStats.Headroom!="applied"{a.appendLog("Headroom "+saverStats.Headroom)}
		cost,err:=a.store.CalculateCost(r.Context(),"opencode",strings.TrimPrefix(model,"oc/"),result.Usage)
		if err!=nil{cost=0}
		entry:=storage.UsageEntry{
			Timestamp:started.UTC().Format(time.RFC3339Nano),Provider:"opencode",Model:model,
			APIKey:presented,Endpoint:r.URL.Path,Status:fmt.Sprint(result.StatusCode),
			Cost:cost,Tokens:result.Usage,Meta:meta,DurationMS:time.Since(started).Milliseconds(),
		}
		saveCtx,cancel:=context.WithTimeout(context.Background(),3*time.Second);defer cancel()
		if err:=a.store.SaveRequestUsage(saveCtx,entry);err!=nil{a.logger.Warn("usage persistence failed","error",err)}
	})(w,r)
}
