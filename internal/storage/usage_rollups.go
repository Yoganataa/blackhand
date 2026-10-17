package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// GetUsageStats uses usageDaily for range aggregates. Raw usageHistory is read
// only for the last ten minutes and recent request rows.
func (s *Store) GetUsageStats(ctx context.Context, period string) (map[string]any, error) {
	if period == "" { period = "all" }
	switch period {
	case "today", "24h", "7d", "30d", "60d", "all":
	default:
		return nil, fmt.Errorf("Invalid period")
	}
	if period == "24h" {
		// A rolling 24-hour window does not align with calendar-day rollups;
		// this implementation is still a timestamp-indexed, bounded query.
		return s.getUsageStatsLegacy(ctx, period)
	}

	now := time.Now()
	localNow := now.In(time.Local)
	start := time.Time{}
	switch period {
	case "today":
		y,m,d := localNow.Date()
		start = time.Date(y,m,d,0,0,0,0,time.Local)
	case "7d":
		start = time.Date(localNow.Year(),localNow.Month(),localNow.Day(),0,0,0,0,time.Local).AddDate(0,0,-6)
	case "30d":
		start = time.Date(localNow.Year(),localNow.Month(),localNow.Day(),0,0,0,0,time.Local).AddDate(0,0,-29)
	case "60d":
		start = time.Date(localNow.Year(),localNow.Month(),localNow.Day(),0,0,0,0,time.Local).AddDate(0,0,-59)
	}
	stats := emptyUsageStats()
	query := "SELECT dateKey,data FROM usageDaily"
	args := []any{}
	if !start.IsZero() {
		query += " WHERE dateKey>=?"
		args = append(args,start.Format("2006-01-02"))
	}
	query += " ORDER BY dateKey ASC"
	rows,err := s.DB.QueryContext(ctx,query,args...)
	if err != nil { return nil,err }
	for rows.Next() {
		var dateKey,raw string
		if err := rows.Scan(&dateKey,&raw); err != nil { rows.Close(); return nil,err }
		day := map[string]any{}
		if err := json.Unmarshal([]byte(raw),&day); err != nil {
			rows.Close()
			return nil,fmt.Errorf("decode usageDaily %s: %w",dateKey,err)
		}
		mergeDailyIntoStats(stats,day)
	}
	if err := rows.Err(); err != nil { rows.Close(); return nil,err }
	if err := rows.Close(); err != nil { return nil,err }

	last10,err := s.getLastTenMinutes(ctx,now)
	if err != nil { return nil,err }
	stats["last10Minutes"] = last10
	recent,err := s.getRecentRequestsSince(ctx,start,20)
	if err != nil { return nil,err }
	stats["recentRequests"] = recent
	return stats,nil
}

func emptyUsageStats() map[string]any {
	return map[string]any{
		"totalRequests":float64(0),
		"totalPromptTokens":float64(0),
		"totalCompletionTokens":float64(0),
		"totalCachedTokens":float64(0),
		"totalCost":float64(0),
		"byProvider":map[string]any{},
		"byModel":map[string]any{},
		"byAccount":map[string]any{},
		"byApiKey":map[string]any{},
		"byEndpoint":map[string]any{},
		"last10Minutes":[]any{},
		"pending":map[string]any{},
		"activeRequests":[]any{},
		"recentRequests":[]any{},
		"errorProvider":"",
	}
}

func mergeDailyIntoStats(stats,day map[string]any) {
	for _,field := range []string{"requests","promptTokens","completionTokens","cachedTokens","cost"} {
		target := map[string]string{
			"requests":"totalRequests",
			"promptTokens":"totalPromptTokens",
			"completionTokens":"totalCompletionTokens",
			"cachedTokens":"totalCachedTokens",
			"cost":"totalCost",
		}[field]
		stats[target] = numberValue(stats[target]) + numberValue(day[field])
	}
	for _,field := range []string{"byProvider","byModel","byAccount","byApiKey","byEndpoint"} {
		mergeUsageMap(usageMap(stats,field),day[field],field)
	}
}

func mergeUsageMap(target map[string]any,raw any,field string) {
	source,ok := raw.(map[string]any)
	if !ok { return }
	for sourceKey,value := range source {
		item,ok := value.(map[string]any)
		if !ok { continue }
		key := sourceKey
		if field == "byModel" {
			model,_ := item["rawModel"].(string)
			provider,_ := item["provider"].(string)
			if model != "" && provider != "" { key = model + " (" + provider + ")" } else if model != "" { key = model }
		}
		existing,ok := target[key].(map[string]any)
		if !ok {
			existing = map[string]any{
				"requests":float64(0),"promptTokens":float64(0),
				"completionTokens":float64(0),"cachedTokens":float64(0),"cost":float64(0),
			}
			target[key] = existing
		}
		for _,counter := range []string{"requests","promptTokens","completionTokens","cachedTokens","cost"} {
			existing[counter] = numberValue(existing[counter]) + numberValue(item[counter])
		}
		for metaKey,metaValue := range item {
			switch metaKey {
			case "requests","promptTokens","completionTokens","cachedTokens","cost":
				continue
			case "lastUsed":
				old,_ := existing[metaKey].(string)
				next,_ := metaValue.(string)
				if next > old { existing[metaKey] = next }
			default:
				if _,has:=existing[metaKey]; !has { existing[metaKey]=metaValue }
			}
		}
		if field == "byApiKey" {
			if sourceKey == "local-no-key" {
				existing["apiKeyMasked"] = nil
				existing["apiKeyKey"] = nil
				existing["keyName"] = "Local (No API Key)"
			} else {
				rawKey := strings.SplitN(sourceKey,"|",2)[0]
				existing["apiKeyMasked"] = maskUsageKey(rawKey)
				existing["apiKeyKey"] = maskUsageKey(rawKey)
				if len(rawKey)>8 { existing["keyName"]=rawKey[:8]+"..." } else { existing["keyName"]=rawKey }
			}
		}
	}
}

func (s *Store) getLastTenMinutes(ctx context.Context,now time.Time)([]any,error) {
	start := time.Unix(now.Unix()/60*60,0).Add(-9*time.Minute)
	buckets := make([]any,10)
	index := map[int64]map[string]any{}
	for i:=0;i<10;i++ {
		at := start.Add(time.Duration(i)*time.Minute)
		b := map[string]any{"timestamp":at.Format(time.RFC3339),"requests":float64(0),"promptTokens":float64(0),"completionTokens":float64(0),"cost":float64(0)}
		buckets[i] = b
		index[at.Unix()/60] = b
	}
	rows,err := s.DB.QueryContext(ctx,"SELECT timestamp,promptTokens,completionTokens,cost FROM usageHistory WHERE timestamp>=? AND timestamp<=? ORDER BY timestamp ASC",start.UTC().Format(time.RFC3339Nano),now.Add(time.Second).UTC().Format(time.RFC3339Nano))
	if err != nil { return nil,err }
	defer rows.Close()
	for rows.Next() {
		var stamp string
		var prompt,completion int64
		var cost float64
		if err:=rows.Scan(&stamp,&prompt,&completion,&cost);err!=nil{return nil,err}
		t,err:=time.Parse(time.RFC3339Nano,stamp)
		if err!=nil{continue}
		b,ok:=index[t.Unix()/60]
		if !ok{continue}
		b["requests"]=numberValue(b["requests"])+1
		b["promptTokens"]=numberValue(b["promptTokens"])+float64(prompt)
		b["completionTokens"]=numberValue(b["completionTokens"])+float64(completion)
		b["cost"]=numberValue(b["cost"])+cost
	}
	return buckets,rows.Err()
}

func (s *Store) getRecentRequestsSince(ctx context.Context,start time.Time,limit int)([]any,error) {
	if limit<=0||limit>100{limit=20}
	query:="SELECT timestamp,provider,model,status,tokens,promptTokens,completionTokens FROM usageHistory"
	args:=[]any{}
	if !start.IsZero(){query+=" WHERE timestamp>=?";args=append(args,start.UTC().Format(time.RFC3339Nano))}
	query+=" ORDER BY id DESC LIMIT 100"
	rows,err:=s.DB.QueryContext(ctx,query,args...)
	if err!=nil{return nil,err}
	defer rows.Close()
	out:=[]any{}
	seen:=map[string]bool{}
	for rows.Next(){
		var stamp,provider,model,status,tokensRaw sql.NullString
		var prompt,completion int64
		if err:=rows.Scan(&stamp,&provider,&model,&status,&tokensRaw,&prompt,&completion);err!=nil{return nil,err}
		tokens:=map[string]any{}
		if tokensRaw.Valid{_ = json.Unmarshal([]byte(tokensRaw.String),&tokens)}
		promptCount:=tokenInt(tokens,"prompt_tokens","input_tokens");if promptCount==0{promptCount=prompt}
		completionCount:=tokenInt(tokens,"completion_tokens","output_tokens");if completionCount==0{completionCount=completion}
		cached:=tokenInt(tokens,"cached_tokens","cache_read_input_tokens")
		if promptCount==0&&completionCount==0{continue}
		key:=fmt.Sprintf("%s|%s|%d|%d|%.16s",model.String,provider.String,promptCount,completionCount,stamp.String)
		if seen[key]{continue};seen[key]=true
		out=append(out,map[string]any{"timestamp":stamp.String,"model":model.String,"provider":provider.String,"promptTokens":promptCount,"completionTokens":completionCount,"cachedTokens":cached,"status":defaultString(status.String,"ok")})
		if len(out)>=limit{break}
	}
	return out,rows.Err()
}

// GetChartData uses rollups for calendar-day ranges. The two hourly periods
// retain a bounded timestamp query because they are not aligned to day buckets.
func (s *Store) GetChartData(ctx context.Context,period string)([]map[string]any,error) {
	if period==""{period="7d"}
	switch period{
	case "today","24h":return s.getChartDataHourlyLegacy(ctx,period)
	case "7d","30d","60d","all":
	default:return nil,fmt.Errorf("Invalid period")
	}
	now:=time.Now().In(time.Local)
	today:=time.Date(now.Year(),now.Month(),now.Day(),0,0,0,0,time.Local)
	start:=today
	switch period{
	case "7d":start=today.AddDate(0,0,-6)
	case "30d":start=today.AddDate(0,0,-29)
	case "60d":start=today.AddDate(0,0,-59)
	case "all":
		var first sql.NullString
		if err:=s.DB.QueryRowContext(ctx,"SELECT MIN(dateKey) FROM usageDaily").Scan(&first);err!=nil{return nil,err}
		if !first.Valid{return []map[string]any{},nil}
		parsed,err:=time.ParseInLocation("2006-01-02",first.String,time.Local)
		if err!=nil{return nil,err}
		start=parsed
	}
	days:=[]time.Time{}
	for d:=start;!d.After(today);d=d.AddDate(0,0,1){
		days=append(days,d)
		if len(days)>10000{return nil,fmt.Errorf("usage chart range exceeds 10000 days")}
	}
	out:=make([]map[string]any,len(days))
	index:=map[string]int{}
	for i,d:=range days{
		key:=d.Format("2006-01-02")
		index[key]=i
		out[i]=map[string]any{"label":d.Format("Jan 2"),"tokens":float64(0),"cost":float64(0),"requests":float64(0)}
	}
	rows,err:=s.DB.QueryContext(ctx,"SELECT dateKey,data FROM usageDaily WHERE dateKey>=? AND dateKey<=? ORDER BY dateKey ASC",start.Format("2006-01-02"),today.Format("2006-01-02"))
	if err!=nil{return nil,err}
	defer rows.Close()
	for rows.Next(){
		var dateKey,raw string
		if err:=rows.Scan(&dateKey,&raw);err!=nil{return nil,err}
		i,ok:=index[dateKey];if !ok{continue}
		day:=map[string]any{}
		if err:=json.Unmarshal([]byte(raw),&day);err!=nil{return nil,fmt.Errorf("decode chart rollup %s: %w",dateKey,err)}
		out[i]["tokens"]=numberValue(day["promptTokens"])+numberValue(day["completionTokens"])
		out[i]["cost"]=numberValue(day["cost"])
		out[i]["requests"]=numberValue(day["requests"])
	}
	return out,rows.Err()
}

// EnsureUsageRollups backfills usageDaily once after a database upgrade. This
// is a one-time scan for migration only; normal dashboard refreshes never scan
// the full request table.
func (s *Store) EnsureUsageRollups(ctx context.Context)error{
	const marker="usageDailyRollupsInitialized"
	done,err:=s.legacyImported(ctx,marker)
	if err!=nil||done{return err}
	var dailyCount,historyCount int64
	if err:=s.DB.QueryRowContext(ctx,"SELECT COUNT(*) FROM usageDaily").Scan(&dailyCount);err!=nil{return err}
	if err:=s.DB.QueryRowContext(ctx,"SELECT COUNT(*) FROM usageHistory").Scan(&historyCount);err!=nil{return err}
	if dailyCount==0&&historyCount>0{
		rows,err:=s.DB.QueryContext(ctx,"SELECT timestamp,provider,model,connectionId,apiKey,endpoint,cost,status,tokens,meta,promptTokens,completionTokens FROM usageHistory ORDER BY id ASC")
		if err!=nil{return err}
		rollups:=map[string]map[string]any{}
		for rows.Next(){
			var stamp,provider,model,conn,key,endpoint,status,tokensRaw,metaRaw sql.NullString
			var cost float64
			var prompt,completion int64
			if err:=rows.Scan(&stamp,&provider,&model,&conn,&key,&endpoint,&cost,&status,&tokensRaw,&metaRaw,&prompt,&completion);err!=nil{rows.Close();return err}
			tokens:=map[string]any{};if tokensRaw.Valid{_ = json.Unmarshal([]byte(tokensRaw.String),&tokens)}
			meta:=map[string]any{};if metaRaw.Valid{_ = json.Unmarshal([]byte(metaRaw.String),&meta)}
			entry:=UsageEntry{Timestamp:stamp.String,Provider:provider.String,Model:model.String,ConnectionID:conn.String,APIKey:key.String,Endpoint:endpoint.String,Cost:cost,Status:status.String,Tokens:tokens,Meta:meta}
			dateKey:=localDateKey(entry.Timestamp)
			day:=rollups[dateKey];if day==nil{day=defaultUsageDay();rollups[dateKey]=day}
			aggregateUsageDay(day,entry,prompt,completion,tokenInt(tokens,"cached_tokens","cache_read_input_tokens"))
		}
		if err:=rows.Err();err!=nil{rows.Close();return err}
		if err:=rows.Close();err!=nil{return err}
		tx,err:=s.DB.BeginTx(ctx,nil);if err!=nil{return err}
		for dateKey,day:=range rollups{
			raw,err:=json.Marshal(day);if err!=nil{_ = tx.Rollback();return err}
			if _,err:=tx.ExecContext(ctx,"INSERT OR IGNORE INTO usageDaily(dateKey,data) VALUES(?,?)",dateKey,string(raw));err!=nil{_ = tx.Rollback();return err}
		}
		if err:=tx.Commit();err!=nil{return err}
	}
	return s.setLegacyImported(ctx,marker)
}
