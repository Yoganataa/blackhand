package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

type UsageEntry struct {
	Timestamp string         `json:"timestamp"`
	Provider string           `json:"provider"`
	Model string              `json:"model"`
	ConnectionID string       `json:"connectionId,omitempty"`
	APIKey string             `json:"apiKey,omitempty"`
	Endpoint string           `json:"endpoint,omitempty"`
	Status string             `json:"status"`
	Cost float64              `json:"cost"`
	Tokens map[string]any     `json:"tokens"`
	Meta map[string]any       `json:"meta,omitempty"`
	DurationMS int64          `json:"durationMs,omitempty"`
}

type UsageFilter struct {
	Provider string
	Model string
	StartDate string
	EndDate string
	Limit int
}

func tokenInt(tokens map[string]any, keys ...string) int64 {
	for _, key := range keys {
		switch v := tokens[key].(type) {
		case float64: return int64(v)
		case int: return int64(v)
		case int64: return v
		case json.Number: n,_:=v.Int64(); return n
		}
	}
	return 0
}
func tokenFloat(tokens map[string]any, keys ...string) float64 {
	for _, key := range keys {
		switch v := tokens[key].(type) {
		case float64: return v
		case int: return float64(v)
		case int64: return float64(v)
		case json.Number: n,_:=v.Float64(); return n
		}
	}
	return 0
}
func maskUsageKey(key string) any {
	if key == "" { return nil }
	if len(key) <= 12 { return key[:1] + "***" }
	return key[:8] + "***" + key[len(key)-4:]
}

func (s *Store) SaveRequestUsage(ctx context.Context, entry UsageEntry) error {
	if entry.Timestamp == "" { entry.Timestamp = nowISO() }
	if entry.Status == "" { entry.Status = "ok" }
	if entry.Tokens == nil { entry.Tokens = map[string]any{} }
	prompt := tokenInt(entry.Tokens, "prompt_tokens", "input_tokens")
	completion := tokenInt(entry.Tokens, "completion_tokens", "output_tokens")
	cached := tokenInt(entry.Tokens, "cached_tokens", "cache_read_input_tokens")
	tokensJSON, err := json.Marshal(entry.Tokens)
	if err != nil { return err }
	metaJSON, err := json.Marshal(entry.Meta)
	if err != nil { return err }
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil { return err }
	defer tx.Rollback()

	// Idempotency guards against format translators reporting the same usage
	// event twice. Different timestamps, endpoints, or token counts remain distinct.
	var existingID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM usageHistory WHERE timestamp=? AND COALESCE(provider,'')=COALESCE(?, '')
		AND COALESCE(model,'')=COALESCE(?, '') AND COALESCE(connectionId,'')=COALESCE(?, '')
		AND COALESCE(apiKey,'')=COALESCE(?, '') AND promptTokens=? AND completionTokens=?
		ORDER BY id DESC LIMIT 1`, entry.Timestamp, nilIfEmpty(entry.Provider), nilIfEmpty(entry.Model), nilIfEmpty(entry.ConnectionID), nilIfEmpty(entry.APIKey), prompt, completion).Scan(&existingID)
	if err == nil {
		var oldEndpoint string
		_ = tx.QueryRowContext(ctx, "SELECT COALESCE(endpoint,'') FROM usageHistory WHERE id=?", existingID).Scan(&oldEndpoint)
		if oldEndpoint == "" && entry.Endpoint != "" {
			if _, err := tx.ExecContext(ctx, "UPDATE usageHistory SET endpoint=? WHERE id=?", entry.Endpoint, existingID); err != nil { return err }
		}
		return tx.Commit()
	}
	if err != sql.ErrNoRows { return err }

	_, err = tx.ExecContext(ctx, `INSERT INTO usageHistory(timestamp,provider,model,connectionId,apiKey,endpoint,promptTokens,completionTokens,cost,status,tokens,meta)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		entry.Timestamp, nilIfEmpty(entry.Provider), nilIfEmpty(entry.Model), nilIfEmpty(entry.ConnectionID), nilIfEmpty(entry.APIKey),
		nilIfEmpty(entry.Endpoint), prompt, completion, entry.Cost, entry.Status, string(tokensJSON), string(metaJSON))
	if err != nil { return err }

	dateKey := localDateKey(entry.Timestamp)
	var dayRaw string
	err = tx.QueryRowContext(ctx, "SELECT data FROM usageDaily WHERE dateKey=?", dateKey).Scan(&dayRaw)
	day := defaultUsageDay()
	if err == nil && dayRaw != "" {
		if err := json.Unmarshal([]byte(dayRaw), &day); err != nil { return fmt.Errorf("decode daily usage %s: %w", dateKey, err) }
	} else if err != nil && err != sql.ErrNoRows { return err }
	aggregateUsageDay(day, entry, prompt, completion, cached)
	encodedDay, err := json.Marshal(day)
	if err != nil { return err }
	if _, err := tx.ExecContext(ctx, `INSERT INTO usageDaily(dateKey,data) VALUES(?,?) ON CONFLICT(dateKey) DO UPDATE SET data=excluded.data`, dateKey, string(encodedDay)); err != nil { return err }

	var total int64
	_ = tx.QueryRowContext(ctx, "SELECT CAST(value AS INTEGER) FROM _meta WHERE key='totalRequestsLifetime'").Scan(&total)
	total++
	if _, err := tx.ExecContext(ctx, `INSERT INTO _meta(key,value) VALUES('totalRequestsLifetime',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, fmt.Sprint(total)); err != nil { return err }
	return tx.Commit()
}

func nilIfEmpty(value string) any {
	if value == "" { return nil }
	return value
}
func localDateKey(timestamp string) string {
	t, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil { t = time.Now() }
	return t.In(time.Local).Format("2006-01-02")
}
func defaultUsageDay() map[string]any {
	return map[string]any{
		"requests":int64(0), "promptTokens":int64(0), "completionTokens":int64(0), "cachedTokens":int64(0), "cost":float64(0),
		"byProvider":map[string]any{}, "byModel":map[string]any{}, "byAccount":map[string]any{}, "byApiKey":map[string]any{}, "byEndpoint":map[string]any{},
	}
}
func usageMap(day map[string]any, key string) map[string]any {
	m, ok := day[key].(map[string]any)
	if ok && m != nil { return m }
	m = map[string]any{}
	day[key] = m
	return m
}
func mapCounter(m map[string]any, key string, prompt, completion, cached int64, cost float64, meta map[string]any) {
	entry, ok := m[key].(map[string]any)
	if !ok { entry = map[string]any{"requests":int64(0),"promptTokens":int64(0),"completionTokens":int64(0),"cachedTokens":int64(0),"cost":float64(0)}; m[key]=entry }
	entry["requests"] = numberValue(entry["requests"]) + 1
	entry["promptTokens"] = numberValue(entry["promptTokens"]) + float64(prompt)
	entry["completionTokens"] = numberValue(entry["completionTokens"]) + float64(completion)
	entry["cachedTokens"] = numberValue(entry["cachedTokens"]) + float64(cached)
	entry["cost"] = numberValue(entry["cost"]) + cost
	for k,v := range meta { entry[k]=v }
}
func numberValue(value any) float64 {
	switch v:=value.(type) {
	case float64: return v
	case int: return float64(v)
	case int64: return float64(v)
	case json.Number: n,_:=v.Float64(); return n
	default: return 0
	}
}
func aggregateUsageDay(day map[string]any, entry UsageEntry, prompt, completion, cached int64) {
	day["requests"] = numberValue(day["requests"]) + 1
	day["promptTokens"] = numberValue(day["promptTokens"]) + float64(prompt)
	day["completionTokens"] = numberValue(day["completionTokens"]) + float64(completion)
	day["cachedTokens"] = numberValue(day["cachedTokens"]) + float64(cached)
	day["cost"] = numberValue(day["cost"]) + entry.Cost
	values := map[string]any{}
	_ = values
	if entry.Provider != "" {
		mapCounter(usageMap(day,"byProvider"),entry.Provider,prompt,completion,cached,entry.Cost,nil)
	}
	modelKey:=entry.Model
	if entry.Provider!="" { modelKey += "|" + entry.Provider }
	mapCounter(usageMap(day,"byModel"),modelKey,prompt,completion,cached,entry.Cost,map[string]any{"rawModel":entry.Model,"provider":entry.Provider})
	if entry.ConnectionID!="" {
		mapCounter(usageMap(day,"byAccount"),entry.ConnectionID,prompt,completion,cached,entry.Cost,map[string]any{"rawModel":entry.Model,"provider":entry.Provider})
	}
	apiKey:=entry.APIKey
	if apiKey=="" { apiKey="local-no-key" }
	apiKeyKey:=apiKey+"|"+entry.Model+"|"+defaultString(entry.Provider,"unknown")
	mapCounter(usageMap(day,"byApiKey"),apiKeyKey,prompt,completion,cached,entry.Cost,map[string]any{"rawModel":entry.Model,"provider":entry.Provider,"apiKey":entry.APIKey})
	endpoint:=defaultString(entry.Endpoint,"Unknown")
	endpointKey:=endpoint+"|"+entry.Model+"|"+defaultString(entry.Provider,"unknown")
	mapCounter(usageMap(day,"byEndpoint"),endpointKey,prompt,completion,cached,entry.Cost,map[string]any{"endpoint":endpoint,"rawModel":entry.Model,"provider":entry.Provider})
}

func (s *Store) GetUsageHistory(ctx context.Context, filter UsageFilter) ([]map[string]any, error) {
	conditions:=[]string{}
	args:=[]any{}
	if filter.Provider!="" { conditions=append(conditions,"provider=?"); args=append(args,filter.Provider) }
	if filter.Model!="" { conditions=append(conditions,"model=?"); args=append(args,filter.Model) }
	if filter.StartDate!="" { t,err:=time.Parse(time.RFC3339,filter.StartDate); if err!=nil { t,err=time.Parse("2006-01-02",filter.StartDate) }; if err!=nil{return nil,fmt.Errorf("invalid startDate")}; conditions=append(conditions,"timestamp>=?"); args=append(args,t.Format(time.RFC3339Nano)) }
	if filter.EndDate!="" { t,err:=time.Parse(time.RFC3339,filter.EndDate); if err!=nil { t,err=time.Parse("2006-01-02",filter.EndDate) }; if err!=nil{return nil,fmt.Errorf("invalid endDate")}; conditions=append(conditions,"timestamp<=?"); args=append(args,t.Format(time.RFC3339Nano)) }
	query:=`SELECT timestamp,provider,model,connectionId,apiKey,endpoint,cost,status,tokens,promptTokens,completionTokens FROM usageHistory`
	if len(conditions)>0 { query+=" WHERE "+strings.Join(conditions," AND ") }
	query+=" ORDER BY id DESC"
	if filter.Limit>0 && filter.Limit<=10000 { query += fmt.Sprintf(" LIMIT %d",filter.Limit) }
	rows,err:=s.DB.QueryContext(ctx,query,args...)
	if err!=nil{return nil,err}
	defer rows.Close()
	out:=[]map[string]any{}
	for rows.Next(){
		var ts,provider,model,conn,key,endpoint,status,tokensRaw sql.NullString
		var cost float64; var prompt,completion int64
		if err:=rows.Scan(&ts,&provider,&model,&conn,&key,&endpoint,&cost,&status,&tokensRaw,&prompt,&completion);err!=nil{return nil,err}
		tokens:=map[string]any{}
		if tokensRaw.Valid { _=json.Unmarshal([]byte(tokensRaw.String),&tokens) }
		out=append(out,map[string]any{"timestamp":ts.String,"provider":nullString(provider),"model":nullString(model),"connectionId":nullString(conn),"apiKeyMasked":maskUsageKey(key.String),"endpoint":nullString(endpoint),"cost":cost,"status":status.String,"tokens":tokens})
	}
	return out,rows.Err()
}

func (s *Store) GetUsageStats(ctx context.Context, period string) (map[string]any, error) {
	if period=="" { period="all" }
	if period!="today" && period!="24h" && period!="7d" && period!="30d" && period!="60d" && period!="all" { return nil,fmt.Errorf("Invalid period") }
	now:=time.Now()
	cutoff:=time.Time{}
	switch period {
	case "today": y,m,d:=now.In(time.Local).Date(); cutoff=time.Date(y,m,d,0,0,0,0,time.Local)
	case "24h": cutoff=now.Add(-24*time.Hour)
	case "7d": cutoff=time.Date(now.Year(),now.Month(),now.Day(),0,0,0,0,time.Local).AddDate(0,0,-6)
	case "30d": cutoff=time.Date(now.Year(),now.Month(),now.Day(),0,0,0,0,time.Local).AddDate(0,0,-29)
	case "60d": cutoff=time.Date(now.Year(),now.Month(),now.Day(),0,0,0,0,time.Local).AddDate(0,0,-59)
	}
	query:=`SELECT timestamp,provider,model,connectionId,apiKey,endpoint,cost,status,tokens,promptTokens,completionTokens FROM usageHistory`
	args:=[]any{}
	if !cutoff.IsZero(){query+=" WHERE timestamp>=?";args=append(args,cutoff.UTC().Format(time.RFC3339Nano))}
	query+=" ORDER BY id DESC"
	rows,err:=s.DB.QueryContext(ctx,query,args...)
	if err!=nil{return nil,err}
	defer rows.Close()

	stats:=map[string]any{
		"totalRequests":int64(0),"totalPromptTokens":int64(0),"totalCompletionTokens":int64(0),"totalCachedTokens":int64(0),"totalCost":float64(0),
		"byProvider":map[string]any{},"byModel":map[string]any{},"byAccount":map[string]any{},"byApiKey":map[string]any{},"byEndpoint":map[string]any{},
		"last10Minutes":[]any{},"pending":map[string]any{},"activeRequests":[]any{},"recentRequests":[]any{},"errorProvider":"",
	}
	prov:=usageMap(stats,"byProvider"); models:=usageMap(stats,"byModel"); keys:=usageMap(stats,"byApiKey"); endpoints:=usageMap(stats,"byEndpoint")
	recent:=[]any{}
	seen:=map[string]bool{}
	last10:=make([]any,10)
	bucketStart:=time.Unix(now.Unix()/60*60,0).Add(-9*time.Minute)
	buckets:=map[int64]map[string]any{}
	for i:=0;i<10;i++{
		t:=bucketStart.Add(time.Duration(i)*time.Minute)
		b:=map[string]any{"timestamp":t.Format(time.RFC3339),"requests":int64(0),"promptTokens":int64(0),"completionTokens":int64(0),"cost":float64(0)}
		last10[i]=b;buckets[t.Unix()/60]=b
	}
	for rows.Next(){
		var ts,provider,model,conn,key,endpoint,status,tokensRaw sql.NullString
		var cost float64;var prompt,completion int64
		if err:=rows.Scan(&ts,&provider,&model,&conn,&key,&endpoint,&cost,&status,&tokensRaw,&prompt,&completion);err!=nil{return nil,err}
		token:=map[string]any{};if tokensRaw.Valid{_ = json.Unmarshal([]byte(tokensRaw.String),&token)}
		cached:=tokenInt(token,"cached_tokens","cache_read_input_tokens")
		prompt2:=tokenInt(token,"prompt_tokens","input_tokens");if prompt2==0{prompt2=prompt}
		completion2:=tokenInt(token,"completion_tokens","output_tokens");if completion2==0{completion2=completion}
		pv,mv,kv,ev:=provider.String,model.String,key.String,endpoint.String
		stats["totalRequests"]=numberValue(stats["totalRequests"])+1
		stats["totalPromptTokens"]=numberValue(stats["totalPromptTokens"])+float64(prompt2)
		stats["totalCompletionTokens"]=numberValue(stats["totalCompletionTokens"])+float64(completion2)
		stats["totalCachedTokens"]=numberValue(stats["totalCachedTokens"])+float64(cached)
		stats["totalCost"]=numberValue(stats["totalCost"])+cost
		mapCounter(prov,defaultString(pv,"unknown"),prompt2,completion2,cached,cost,nil)
		modelKey:=defaultString(mv,"unknown");if pv!=""{modelKey+=" ("+pv+")"}
		mapCounter(models,modelKey,prompt2,completion2,cached,cost,map[string]any{"rawModel":mv,"provider":pv,"lastUsed":ts.String})
		apiKeyMasked:=maskUsageKey(kv)
		apiKeyKey:=kv+"|"+mv+"|"+defaultString(pv,"unknown")
		if kv==""{apiKeyKey="local-no-key"}
		mapCounter(keys,apiKeyKey,prompt2,completion2,cached,cost,map[string]any{"rawModel":mv,"provider":pv,"apiKeyMasked":apiKeyMasked,"apiKeyKey":apiKeyMasked,"keyName":map[bool]string{true:"Local (No API Key)",false:func()string{if len(kv)>8{return kv[:8]+"..."};return kv}()}[kv==""] ,"lastUsed":ts.String})
		ep:=defaultString(ev,"Unknown")
		mapCounter(endpoints,ep+"|"+mv+"|"+defaultString(pv,"unknown"),prompt2,completion2,cached,cost,map[string]any{"endpoint":ep,"rawModel":mv,"provider":pv,"lastUsed":ts.String})
		if t,err:=time.Parse(time.RFC3339Nano,ts.String);err==nil {
			bucket:=t.Unix()/60
			if b,ok:=buckets[bucket];ok{b["requests"]=numberValue(b["requests"])+1;b["promptTokens"]=numberValue(b["promptTokens"])+float64(prompt2);b["completionTokens"]=numberValue(b["completionTokens"])+float64(completion2);b["cost"]=numberValue(b["cost"])+cost}
		}
		if len(recent)<100 {
			item:=map[string]any{"timestamp":ts.String,"model":mv,"provider":pv,"promptTokens":prompt2,"completionTokens":completion2,"cachedTokens":cached,"status":defaultString(status.String,"ok")}
			if prompt2>0||completion2>0{
				key:=fmt.Sprintf("%v|%v|%d|%d|%.16s",mv,pv,prompt2,completion2,ts.String)
				if !seen[key]{seen[key]=true;recent=append(recent,item)}
			}
		}
	}
	if err:=rows.Err();err!=nil{return nil,err}
	stats["last10Minutes"]=last10
	if len(recent)>20{recent=recent[:20]}
	stats["recentRequests"]=recent
	return stats,nil
}

func (s *Store) GetChartData(ctx context.Context, period string) ([]map[string]any, error) {
	if period==""{period="7d"}
	if period!="today"&&period!="24h"&&period!="7d"&&period!="30d"&&period!="60d"&&period!="all"{return nil,fmt.Errorf("Invalid period")}
	now:=time.Now(); start:=now
	bucketMs:=time.Hour; count:=24
	switch period{
	case "today": y,m,d:=now.In(time.Local).Date();start=time.Date(y,m,d,0,0,0,0,time.Local);count=24
	case "24h": start=now.Add(-24*time.Hour);count=24
	case "7d": count=7;start=time.Date(now.Year(),now.Month(),now.Day(),0,0,0,0,time.Local).AddDate(0,0,-6);bucketMs=24*time.Hour
	case "30d": count=30;start=time.Date(now.Year(),now.Month(),now.Day(),0,0,0,0,time.Local).AddDate(0,0,-29);bucketMs=24*time.Hour
	case "60d": count=60;start=time.Date(now.Year(),now.Month(),now.Day(),0,0,0,0,time.Local).AddDate(0,0,-59);bucketMs=24*time.Hour
	case "all":
		var first sql.NullString
		if err:=s.DB.QueryRowContext(ctx,"SELECT MIN(timestamp) FROM usageHistory").Scan(&first);err!=nil{return nil,err}
		if !first.Valid{return []map[string]any{},nil}
		t,err:=time.Parse(time.RFC3339Nano,first.String);if err!=nil{return []map[string]any{},nil}
		start=time.Date(t.In(time.Local).Year(),t.In(time.Local).Month(),t.In(time.Local).Day(),0,0,0,0,time.Local)
		count=int(now.In(time.Local).Sub(start)/(24*time.Hour))+1
		if count<1{count=1};bucketMs=24*time.Hour
	}
	out:=make([]map[string]any,count)
	for i:=0;i<count;i++{
		t:=start.Add(time.Duration(i)*bucketMs)
		label:=t.In(time.Local).Format("Jan 2")
		if period=="today"||period=="24h"{label=t.In(time.Local).Format("15:04")}
		out[i]=map[string]any{"label":label,"tokens":int64(0),"cost":float64(0),"requests":int64(0)}
	}
	end:=now.Add(time.Second)
	rows,err:=s.DB.QueryContext(ctx,"SELECT timestamp,promptTokens,completionTokens,cost FROM usageHistory WHERE timestamp>=? AND timestamp<?",start.UTC().Format(time.RFC3339Nano),end.UTC().Format(time.RFC3339Nano))
	if err!=nil{return nil,err};defer rows.Close()
	for rows.Next(){
		var ts string;var p,c int64;var cost float64
		if err:=rows.Scan(&ts,&p,&c,&cost);err!=nil{return nil,err}
		t,err:=time.Parse(time.RFC3339Nano,ts);if err!=nil{continue}
		idx:=int(t.Sub(start)/bucketMs)
		if idx<0||idx>=len(out){continue}
		out[idx]["tokens"]=numberValue(out[idx]["tokens"])+float64(p+c)
		out[idx]["cost"]=numberValue(out[idx]["cost"])+cost
		out[idx]["requests"]=numberValue(out[idx]["requests"])+1
	}
	return out,rows.Err()
}

func (s *Store) GetRecentUsageLogs(ctx context.Context, limit int) ([]map[string]any, error) {
	if limit<=0||limit>1000{limit=200}
	rows,err:=s.DB.QueryContext(ctx,`SELECT timestamp,provider,model,status,promptTokens,completionTokens,cost FROM usageHistory ORDER BY id DESC LIMIT ?`,limit)
	if err!=nil{return nil,err};defer rows.Close()
	out:=[]map[string]any{}
	for rows.Next(){
		var timestamp,provider,model,status sql.NullString;var prompt,completion int64;var cost float64
		if err:=rows.Scan(&timestamp,&provider,&model,&status,&prompt,&completion,&cost);err!=nil{return nil,err}
		out=append(out,map[string]any{"timestamp":timestamp.String,"provider":provider.String,"model":model.String,"status":status.String,"promptTokens":prompt,"completionTokens":completion,"cost":cost})
	}
	return out,rows.Err()
}

func roundCost(value float64) float64 { return math.Round(value*1e8)/1e8 }


// CalculateCost uses user-defined 9Router pricing overrides stored in kv(scope=pricing).
// OpenCode Free models without an explicit override are billed at zero.
func (s *Store) CalculateCost(ctx context.Context, provider, model string, tokens map[string]any) (float64, error) {
	var raw string
	err := s.DB.QueryRowContext(ctx, "SELECT value FROM kv WHERE scope='pricing' AND key=?", provider).Scan(&raw)
	if err != nil {
		if err == sql.ErrNoRows { return 0, nil }
		return 0, err
	}
	models := map[string]map[string]any{}
	if err := json.Unmarshal([]byte(raw), &models); err != nil { return 0, err }
	pricing := models[model]
	if pricing == nil { pricing = models[strings.TrimPrefix(model, "oc/")] }
	if pricing == nil { return 0, nil }
	rate := func(key string, fallback float64) float64 {
		if value, ok := pricing[key]; ok { return numberValue(value) }
		return fallback
	}
	input := float64(tokenInt(tokens, "prompt_tokens", "input_tokens"))
	cached := float64(tokenInt(tokens, "cached_tokens", "cache_read_input_tokens"))
	cacheCreation := float64(tokenInt(tokens, "cache_creation_input_tokens"))
	nonCached := math.Max(0, input-cached-cacheCreation)
	cost := nonCached * rate("input", 0) / 1_000_000
	if cached > 0 { cost += cached * rate("cached", rate("input", 0)) / 1_000_000 }
	cost += float64(tokenInt(tokens, "completion_tokens", "output_tokens")) * rate("output", 0) / 1_000_000
	reasoning := float64(tokenInt(tokens, "reasoning_tokens"))
	if reasoning > 0 { cost += reasoning * rate("reasoning", rate("output", 0)) / 1_000_000 }
	if cacheCreation > 0 { cost += cacheCreation * rate("cache_creation", rate("input", 0)) / 1_000_000 }
	return roundCost(cost), nil
}
