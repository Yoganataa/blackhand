package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Yoganataa/blackhand/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

func decodeJSON(w http.ResponseWriter,r *http.Request,dst any,maxBytes int64)error{
	r.Body=http.MaxBytesReader(w,r.Body,maxBytes)
	decoder:=json.NewDecoder(r.Body)
	if err:=decoder.Decode(dst);err!=nil{return err}
	var extra any
	if err:=decoder.Decode(&extra);err!=io.EOF{return fmt.Errorf("request must contain one JSON value")}
	return nil
}

func (a *API) keysList(w http.ResponseWriter,r *http.Request){
	keys,err:=a.store.GetAPIKeys(r.Context());if err!=nil{writeJSON(w,500,map[string]string{"error":"Failed to fetch keys"});return}
	writeJSON(w,200,map[string]any{"keys":keys})
}
func (a *API) keysCreate(w http.ResponseWriter,r *http.Request){
	var body struct{Name string `json:"name"`}
	if err:=decodeJSON(w,r,&body,64<<10);err!=nil{writeJSON(w,400,map[string]string{"error":"Invalid JSON"});return}
	key,err:=a.store.CreateAPIKey(r.Context(),body.Name)
	if err!=nil{writeJSON(w,400,map[string]string{"error":err.Error()});return}
	a.appendLog("API key created: "+key.ID)
	writeJSON(w,201,map[string]any{"key":key.Key,"name":key.Name,"id":key.ID,"machineId":key.MachineID})
}
func (a *API) keyGet(w http.ResponseWriter,r *http.Request){
	key,err:=a.store.GetAPIKeyByID(r.Context(),r.PathValue("id"));if err!=nil{writeJSON(w,500,map[string]string{"error":"Failed to fetch key"});return};if key==nil{writeJSON(w,404,map[string]string{"error":"Key not found"});return};writeJSON(w,200,map[string]any{"key":key})
}
func (a *API) keyUpdate(w http.ResponseWriter,r *http.Request){
	var body map[string]json.RawMessage
	if err:=decodeJSON(w,r,&body,64<<10);err!=nil{writeJSON(w,400,map[string]string{"error":"Invalid JSON"});return}
	var active *bool
	if raw,ok:=body["isActive"];ok{var v bool;if err:=json.Unmarshal(raw,&v);err!=nil{writeJSON(w,400,map[string]string{"error":"isActive must be a boolean"});return};active=&v}
	var access *storage.KeyAccess
	if raw,ok:=body["access"];ok{
		var v storage.KeyAccess;if err:=json.Unmarshal(raw,&v);err!=nil{writeJSON(w,400,map[string]string{"error":"Invalid access object"});return}
		validated,err:=storage.ValidateKeyAccess(v);if err!=nil{writeJSON(w,400,map[string]string{"error":err.Error()});return};access=&validated
	}
	key,err:=a.store.UpdateAPIKey(r.Context(),r.PathValue("id"),active,access)
	if err!=nil{writeJSON(w,500,map[string]string{"error":"Failed to update key"});return};if key==nil{writeJSON(w,404,map[string]string{"error":"Key not found"});return}
	writeJSON(w,200,map[string]any{"key":key})
}
func (a *API) keyDelete(w http.ResponseWriter,r *http.Request){
	id:=r.PathValue("id");removed,err:=a.store.DeleteAPIKey(r.Context(),id)
	if err!=nil{writeJSON(w,500,map[string]string{"error":"Failed to delete key"});return};if !removed{writeJSON(w,404,map[string]string{"error":"Key not found"});return}
	a.appendLog("API key deleted: "+id);writeJSON(w,200,map[string]string{"message":"Key deleted successfully"})
}

func (a *API) settingsGet(w http.ResponseWriter,r *http.Request){
	settings,err:=a.store.GetSettings(r.Context());if err!=nil{writeJSON(w,500,map[string]string{"error":"Error getting settings"});return}
	_,hasPassword:=settings["password"];secret,_:=settings["oidcClientSecret"].(string)
	delete(settings,"password");delete(settings,"oidcClientSecret")
	settings["oidcConfigured"]=settings["oidcIssuerUrl"]!=""&&settings["oidcClientId"]!=""&&secret!=""
	settings["enableRequestLogs"]=strings.EqualFold(strings.TrimSpace(getEnv("ENABLE_REQUEST_LOGS")),"true")
	settings["enableTranslator"]=strings.EqualFold(strings.TrimSpace(getEnv("ENABLE_TRANSLATOR")),"true")
	settings["hasPassword"]=hasPassword
	writeJSON(w,200,settings)
}
func (a *API) settingsPatch(w http.ResponseWriter,r *http.Request){
	var body map[string]any
	if err:=decodeJSON(w,r,&body,1<<20);err!=nil{writeJSON(w,400,map[string]string{"error":"Invalid JSON"});return}
	delete(body,"password");delete(body,"mitmSudoEncrypted")
	var passwordHash string
	if value,ok:=body["newPassword"];ok&&fmt.Sprint(value)!=""{
		newPassword,validType:=value.(string);if !validType||len(newPassword)<1||len(newPassword)>1024{writeJSON(w,400,map[string]string{"error":"newPassword must be a string under 1024 bytes"});return}
		settings,err:=a.store.GetSettings(r.Context());if err!=nil{writeJSON(w,500,map[string]string{"error":"settings unavailable"});return}
		currentHash,_:=settings["password"].(string)
		currentPassword,_:=body["currentPassword"].(string)
		if currentHash!=""{
			if currentPassword==""{writeJSON(w,400,map[string]string{"error":"Current password required"});return}
			if bcrypt.CompareHashAndPassword([]byte(currentHash),[]byte(currentPassword))!=nil{writeJSON(w,401,map[string]string{"error":"Invalid current password"});return}
		}else{
			initial:=getEnv("INITIAL_PASSWORD");if initial==""{initial="123456"}
			if currentPassword!=""&&currentPassword!=initial{writeJSON(w,401,map[string]string{"error":"Invalid current password"});return}
		}
		hashed,err:=bcrypt.GenerateFromPassword([]byte(newPassword),bcrypt.DefaultCost);if err!=nil{writeJSON(w,500,map[string]string{"error":"Could not hash password"});return};passwordHash=string(hashed)
		delete(body,"newPassword");delete(body,"currentPassword")
	}
	if value,ok:=body["oidcClientSecret"];ok&&strings.TrimSpace(fmt.Sprint(value))==""{delete(body,"oidcClientSecret")}
	settings,err:=a.store.UpdateSettings(r.Context(),body);if err!=nil{writeJSON(w,500,map[string]string{"error":"Error updating settings"});return}
	if passwordHash!=""{if err:=a.store.SetPasswordHash(r.Context(),passwordHash);err!=nil{writeJSON(w,500,map[string]string{"error":"Settings saved but password update failed"});return};settings,err=a.store.GetSettings(r.Context());if err!=nil{writeJSON(w,500,map[string]string{"error":"Settings saved but could not reload settings"});return}}
	a.appendLog("Settings updated")
	_,hasPassword:=settings["password"];secret,_:=settings["oidcClientSecret"].(string);delete(settings,"password");delete(settings,"oidcClientSecret")
	settings["oidcConfigured"]=settings["oidcIssuerUrl"]!=""&&settings["oidcClientId"]!=""&&secret!=""
	settings["enableRequestLogs"]=strings.EqualFold(getEnv("ENABLE_REQUEST_LOGS"),"true");settings["enableTranslator"]=strings.EqualFold(getEnv("ENABLE_TRANSLATOR"),"true");settings["hasPassword"]=hasPassword
	writeJSON(w,200,settings)
}
func getEnv(k string) string { return strings.TrimSpace(os.Getenv(k)) }

func (a *API) proxyPoolsList(w http.ResponseWriter,r *http.Request){
	var active *bool
	switch r.URL.Query().Get("isActive"){case "true":v:=true;active=&v;case "false":v:=false;active=&v}
	pools,err:=a.store.GetProxyPools(r.Context(),active);if err!=nil{writeJSON(w,500,map[string]string{"error":"Failed to fetch proxy pools"});return}
	if r.URL.Query().Get("includeUsage")=="true"{for _,pool:=range pools{count,err:=a.store.CountBoundConnections(r.Context(),fmt.Sprint(pool["id"]));if err==nil{pool["boundConnectionCount"]=count}}}
	writeJSON(w,200,map[string]any{"proxyPools":pools})
}
func (a *API) proxyPoolCreate(w http.ResponseWriter,r *http.Request){
	var body map[string]any;if err:=decodeJSON(w,r,&body,128<<10);err!=nil{writeJSON(w,400,map[string]string{"error":"Invalid JSON"});return}
	pool,err:=a.store.CreateProxyPool(r.Context(),body);if err!=nil{writeJSON(w,400,map[string]string{"error":err.Error()});return};a.appendLog("Proxy pool created: "+fmt.Sprint(pool["id"]));writeJSON(w,201,map[string]any{"proxyPool":pool})
}
func (a *API) proxyPoolGet(w http.ResponseWriter,r *http.Request){
	pool,err:=a.store.GetProxyPool(r.Context(),r.PathValue("id"));if err!=nil{writeJSON(w,500,map[string]string{"error":"Failed to fetch proxy pool"});return};if pool==nil{writeJSON(w,404,map[string]string{"error":"Proxy pool not found"});return};writeJSON(w,200,map[string]any{"proxyPool":pool})
}
func (a *API) proxyPoolUpdate(w http.ResponseWriter,r *http.Request){
	var body map[string]any;if err:=decodeJSON(w,r,&body,128<<10);err!=nil{writeJSON(w,400,map[string]string{"error":"Invalid JSON"});return}
	pool,err:=a.store.UpdateProxyPool(r.Context(),r.PathValue("id"),body);if err!=nil{writeJSON(w,400,map[string]string{"error":err.Error()});return};if pool==nil{writeJSON(w,404,map[string]string{"error":"Proxy pool not found"});return};writeJSON(w,200,map[string]any{"proxyPool":pool})
}
func (a *API) proxyPoolDelete(w http.ResponseWriter,r *http.Request){
	id:=r.PathValue("id");pool,err:=a.store.GetProxyPool(r.Context(),id);if err!=nil{writeJSON(w,500,map[string]string{"error":"Failed to fetch proxy pool"});return};if pool==nil{writeJSON(w,404,map[string]string{"error":"Proxy pool not found"});return}
	count,err:=a.store.CountBoundConnections(r.Context(),id);if err!=nil{writeJSON(w,500,map[string]string{"error":"Could not verify proxy pool usage"});return};if count>0{writeJSON(w,409,map[string]any{"error":"Proxy pool is currently in use","boundConnectionCount":count});return}
	if _,err:=a.store.DeleteProxyPool(r.Context(),id);err!=nil{writeJSON(w,500,map[string]string{"error":"Failed to delete proxy pool"});return};a.appendLog("Proxy pool deleted: "+id);writeJSON(w,200,map[string]bool{"success":true})
}
func (a *API) proxyPoolTest(w http.ResponseWriter,r *http.Request){
	id:=r.PathValue("id");pool,err:=a.store.GetProxyPool(r.Context(),id);if err!=nil{writeJSON(w,500,map[string]string{"error":"Failed to fetch proxy pool"});return};if pool==nil{writeJSON(w,404,map[string]string{"error":"Proxy pool not found"});return}
	result:=testProxyPool(r.Context(),pool)
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	_,_ = a.store.UpdateProxyPool(r.Context(),id,map[string]any{"testStatus":map[bool]string{true:"active",false:"error"}[result.OK],"lastTestedAt":now,"lastError":result.Error,"isActive":result.OK})
	result.TestedAt=now
	writeJSON(w,200,result)
}

func (a *API) usageHistory(w http.ResponseWriter,r *http.Request){stats,err:=a.store.GetUsageStats(r.Context(),"all");if err!=nil{writeJSON(w,500,map[string]string{"error":"Failed to fetch usage stats"});return};a.attachActiveRequests(stats);writeJSON(w,200,stats)}
func (a *API) usageStats(w http.ResponseWriter,r *http.Request){period:=r.URL.Query().Get("period");if period==""{period="7d"};if !validPeriod(period){writeJSON(w,400,map[string]string{"error":"Invalid period"});return};stats,err:=a.store.GetUsageStats(r.Context(),period);if err!=nil{writeJSON(w,500,map[string]string{"error":"Failed to fetch usage stats"});return};a.attachActiveRequests(stats);writeJSON(w,200,stats)}
func (a *API) usageChart(w http.ResponseWriter,r *http.Request){period:=r.URL.Query().Get("period");if period==""{period="7d"};if !validPeriod(period){writeJSON(w,400,map[string]string{"error":"Invalid period"});return};data,err:=a.store.GetChartData(r.Context(),period);if err!=nil{writeJSON(w,500,map[string]string{"error":"Failed to fetch chart data"});return};writeJSON(w,200,data)}
func (a *API) usageLogs(w http.ResponseWriter,r *http.Request){logs,err:=a.store.GetRecentUsageLogs(r.Context(),200);if err!=nil{writeJSON(w,500,map[string]string{"error":"Failed to fetch logs"});return};writeJSON(w,200,logs)}
func validPeriod(s string)bool{switch s{case "today","24h","7d","30d","60d","all":return true};return false}
func (a *API) beginRequest(model string){a.activeMu.Lock();a.active[model]++;a.activeMu.Unlock()}
func (a *API) endRequest(model string){a.activeMu.Lock();a.active[model]--;if a.active[model]<=0{delete(a.active,model)};a.activeMu.Unlock()}
func (a *API) attachActiveRequests(stats map[string]any){a.activeMu.Lock();defer a.activeMu.Unlock();items:=[]map[string]any{};for model,count:=range a.active{items=append(items,map[string]any{"model":model,"provider":"opencode","account":"OpenCode Free","count":count})};stats["activeRequests"]=items}
func (a *API) usageStream(w http.ResponseWriter,r *http.Request){
	w.Header().Set("Content-Type","text/event-stream");w.Header().Set("Cache-Control","no-cache, no-transform");w.Header().Set("Connection","keep-alive");w.Header().Set("X-Accel-Buffering","no")
	flusher,ok:=w.(http.Flusher);if !ok{http.Error(w,"streaming not supported",500);return}
	send:=func(){stats,err:=a.store.GetUsageStats(r.Context(),"all");if err!=nil{return};a.attachActiveRequests(stats);if raw,err:=json.Marshal(stats);err==nil{_,_=fmt.Fprintf(w,"data: %s\n\n",raw);flusher.Flush()}}
	send();ticker:=time.NewTicker(3*time.Second);defer ticker.Stop()
	for{select{case<-r.Context().Done():return;case<-ticker.C:send()}}
}
func (a *API) consoleLogsGet(w http.ResponseWriter,_ *http.Request){a.logMu.Lock();logs:=append([]string(nil),a.logs...);a.logMu.Unlock();writeJSON(w,200,map[string]any{"success":true,"logs":logs})}
func (a *API) consoleLogsClear(w http.ResponseWriter,_ *http.Request){a.logMu.Lock();a.logs=[]string{};for _,sub:=range a.logSubs{select{case sub<-logEvent{Type:"clear"}:default:}};a.logMu.Unlock();writeJSON(w,200,map[string]bool{"success":true})}
func (a *API) appendLog(line string){
	if len(line)>2000{line=line[:2000]+"…"}
	a.logMu.Lock();a.logs=append(a.logs,line);if len(a.logs)>500{a.logs=append([]string(nil),a.logs[len(a.logs)-500:]...)}
	for _,sub:=range a.logSubs{select{case sub<-logEvent{Type:"line",Line:line}:default:select{case<-sub:default:};select{case sub<-logEvent{Type:"line",Line:line}:default:}}};a.logMu.Unlock()
}
func (a *API) subscribeLogs()(uint64,chan logEvent,[]string){a.logMu.Lock();defer a.logMu.Unlock();a.nextSub++;id:=a.nextSub;ch:=make(chan logEvent,64);a.logSubs[id]=ch;return id,ch,append([]string(nil),a.logs...)}
func (a *API) unsubscribeLogs(id uint64){a.logMu.Lock();delete(a.logSubs,id);a.logMu.Unlock()}
func (a *API) consoleLogsStream(w http.ResponseWriter,r *http.Request){
	w.Header().Set("Content-Type","text/event-stream");w.Header().Set("Cache-Control","no-cache, no-transform");w.Header().Set("Connection","keep-alive");w.Header().Set("X-Accel-Buffering","no")
	flusher,ok:=w.(http.Flusher);if !ok{http.Error(w,"streaming not supported",500);return}
	id,ch,logs:=a.subscribeLogs();defer a.unsubscribeLogs(id)
	if len(logs)>0{raw,_:=json.Marshal(map[string]any{"type":"init","logs":logs});_,_=fmt.Fprintf(w,"data: %s\n\n",raw);flusher.Flush()}
	ping:=time.NewTicker(25*time.Second);defer ping.Stop()
	for{select{case<-r.Context().Done():return;case event:=<-ch:raw,_:=json.Marshal(map[string]any{"type":event.Type,"line":event.Line});_,_=fmt.Fprintf(w,"data: %s\n\n",raw);flusher.Flush();case<-ping.C:_,_=io.WriteString(w,": ping\n\n");flusher.Flush()}}
}

