package tokensaver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const minCompressBytes = 500
const maxRawBytes = 10 << 20

type Hit struct {
	Shape string `json:"shape"`
	Filter string `json:"filter"`
	Saved int `json:"saved"`
}
type Stats struct {
	BytesBefore int `json:"bytesBefore"`
	BytesAfter int `json:"bytesAfter"`
	Hits []Hit `json:"hits"`
	Headroom string `json:"headroom,omitempty"`
	PXPipe string `json:"pxpipe,omitempty"`
}
type Processor struct { client *http.Client; mu sync.Mutex; circuitUntil time.Time }
func New() *Processor { return &Processor{client:&http.Client{Timeout:10*time.Second}} }

func (p *Processor) Process(ctx context.Context, body map[string]any, settings map[string]any, model string) Stats {
	stats:=Stats{Hits:[]Hit{}}
	if enabled(settings["rtkEnabled"],true) { stats=compressMessages(body) }
	if enabled(settings["headroomEnabled"],true) {
		result,reason:=p.headroom(ctx,body,settings,model); stats.Headroom=reason
		if result!=nil { if messages,ok:=result["messages"].([]any);ok { body["messages"]=messages } }
	}
	if enabled(settings["cavemanEnabled"],false) { injectSystemPrompt(body,cavemanPrompt(strValue(settings,"cavemanLevel","full"))) }
	if enabled(settings["ponytailEnabled"],true) { injectSystemPrompt(body,ponytailPrompt(strValue(settings,"ponytailLevel","full"))) }
	if enabled(settings["pxpipeEnabled"],false) { stats.PXPipe="skipped: PXPipe helper is not bundled; request preserved" }
	return stats
}
func enabled(v any, fallback bool) bool { if v==nil{return fallback}; b,ok:=v.(bool);if ok{return b};return fmt.Sprint(v)=="true"||fmt.Sprint(v)=="1" }
func strValue(m map[string]any,k,d string)string{if v,ok:=m[k].(string);ok&&v!=""{return v};return d}
func intValue(m map[string]any,k string,d int)int{switch v:=m[k].(type){case float64:if v>0{return int(v)};case int:if v>0{return v}};return d}

func (p *Processor) headroom(ctx context.Context, body map[string]any, settings map[string]any, model string)(map[string]any,string){
	messages,ok:=body["messages"].([]any);if !ok{return nil,"skipped: unsupported message shape"}
	p.mu.Lock();if time.Now().Before(p.circuitUntil){p.mu.Unlock();return nil,"skipped: circuit open after recent failure"};p.mu.Unlock()
	timeout:=intValue(settings,"headroomTimeoutMs",10000);if timeout>32000{timeout=32000}
	payload:=map[string]any{"messages":messages,"model":model}
	if enabled(settings["headroomCompressUserMessages"],true){payload["config"]=map[string]any{"compress_user_messages":true}}
	raw,err:=json.Marshal(payload);if err!=nil{return nil,"skipped: encode failure"}
	if len(raw)>1_000_000&&timeout<32000{timeout=32000}else if len(raw)>250000&&timeout<15000{timeout=15000}
	url:=strings.TrimRight(strValue(settings,"headroomUrl","http://localhost:8787"),"/")
	if !strings.HasSuffix(url,"/v1/compress"){url+="/v1/compress"}
	reqCtx,cancel:=context.WithTimeout(ctx,time.Duration(timeout)*time.Millisecond);defer cancel()
	req,err:=http.NewRequestWithContext(reqCtx,http.MethodPost,url,bytes.NewReader(raw));if err!=nil{return nil,"skipped: invalid URL"};req.Header.Set("Content-Type","application/json")
	res,err:=p.client.Do(req);if err!=nil{p.mu.Lock();p.circuitUntil=time.Now().Add(30*time.Second);p.mu.Unlock();return nil,"skipped: unavailable; fail-open"}
	defer res.Body.Close()
	if res.StatusCode<200||res.StatusCode>=300{_,_=io.Copy(io.Discard,io.LimitReader(res.Body,1024));p.mu.Lock();p.circuitUntil=time.Now().Add(30*time.Second);p.mu.Unlock();return nil,fmt.Sprintf("skipped: HTTP %d; fail-open",res.StatusCode)}
	var decoded map[string]any;if err:=json.NewDecoder(io.LimitReader(res.Body,16<<20)).Decode(&decoded);err!=nil{return nil,"skipped: invalid response"}
	if _,ok:=decoded["messages"].([]any);!ok{return nil,"skipped: response lacks messages"}
	p.mu.Lock();p.circuitUntil=time.Time{};p.mu.Unlock();return decoded,"applied"
}

func injectSystemPrompt(body map[string]any,prompt string){
	if existing,ok:=body["system"].(string);ok{body["system"]=existing+"

"+prompt;return}
	items,ok:=body["messages"].([]any);if !ok{return}
	for _,item:=range items{msg,ok:=item.(map[string]any);if !ok||msg["role"]!="system"{continue};if current,ok:=msg["content"].(string);ok{msg["content"]=current+"

"+prompt;return}}
	body["messages"]=append([]any{map[string]any{"role":"system","content":prompt}},items...)
}
func cavemanPrompt(level string)string{switch strings.ToLower(level){case "lite":return "Use concise, clear prose. Remove filler but preserve exact technical detail, code, paths, commands, URLs, errors and ordered steps. Use unambiguous language for safety-critical instructions.";case "ultra":return "Use very terse prose for ordinary explanation. Keep all technical identifiers exact; write security warnings and irreversible actions clearly.";case "wenyan","wenyan-lite","wenyan-ultra":return "Prefer compact classical-Chinese-style prose for noncritical explanation. Preserve code and technical identifiers verbatim; use clear language for safety-critical directions."};return "Use terse, direct prose without filler. Preserve the exact meaning of technical content, code, paths, commands, URLs, errors and safety warnings."}
func ponytailPrompt(level string)string{if strings.EqualFold(level,"ultra"){return "Apply YAGNI rigorously. Prefer deletion, standard library and native features. Keep the smallest correct implementation; never omit validation, error handling, security, accessibility or requested behavior."};if strings.EqualFold(level,"lite"){return "Prefer the simplest correct implementation. Avoid unneeded abstractions, dependencies and files. Never skip trust-boundary validation or essential error handling."};return "Act like an efficient senior developer. Prefer the simplest correct implementation, standard library and native features. Avoid unrequested abstractions and dependencies without simplifying away required behavior."}

func compressMessages(body map[string]any)Stats{
	stats:=Stats{Hits:[]Hit{}}
	items,ok:=body["messages"].([]any);if !ok{items,ok=body["input"].([]any)};if !ok{return stats}
	for _,raw:=range items{
		msg,ok:=raw.(map[string]any);if !ok{continue}
		if msg["type"]=="function_call_output"{if text,ok:=msg["output"].(string);ok{msg["output"]=compressText(text,"openai-responses-string",&stats)};continue}
		if msg["role"]=="tool"{if text,ok:=msg["content"].(string);ok{msg["content"]=compressText(text,"openai-tool",&stats);continue}}
		parts,ok:=msg["content"].([]any);if !ok{continue}
		if msg["role"]=="tool"{for _,p:=range parts{if block,ok:=p.(map[string]any);ok&&block["type"]=="text"{if text,ok:=block["text"].(string);ok{block["text"]=compressText(text,"openai-tool-array",&stats)}}};continue}
		for _,part:=range parts{
			block,ok:=part.(map[string]any);if !ok||block["type"]!="tool_result"||block["is_error"]==true{continue}
			if text,ok:=block["content"].(string);ok{block["content"]=compressText(text,"claude-string",&stats);continue}
			if children,ok:=block["content"].([]any);ok{for _,child:=range children{if t,ok:=child.(map[string]any);ok&&t["type"]=="text"{if text,ok:=t["text"].(string);ok{t["text"]=compressText(text,"claude-array",&stats)}}}}
		}
	}
	return stats
}
func compressText(input,shape string,stats *Stats)string{
	before:=len([]byte(input));stats.BytesBefore+=before
	if before<minCompressBytes||before>maxRawBytes{stats.BytesAfter+=before;return input}
	filter:=detect(input);if filter==""{stats.BytesAfter+=before;return input}
	output:=filterText(filter,input);after:=len([]byte(output))
	if output==""||after>=before{stats.BytesAfter+=before;return input}
	stats.BytesAfter+=after;stats.Hits=append(stats.Hits,Hit{shape,filter,before-after});return output
}

var (
	rGitLog=regexp.MustCompile(`(?m)^[*|/\\ ]*commit [0-9a-f]{7,40}$`)
	rGitDiff=regexp.MustCompile(`(?m)^diff --git `)
	rGitHunk=regexp.MustCompile(`(?m)^@@ `)
	rGitStatus=regexp.MustCompile(`(?m)^On branch |^nothing to commit|^Changes (not |to be )|^Untracked files:`)
	rPorcelain=regexp.MustCompile(`(?m)^[ MADRCU?!][ MADRCU?!] \S`)
	rBuild=regexp.MustCompile(`(?im)^(npm (warn|error|ERR!)|yarn (warn|error)|\s*Compiling\s+\S+|\s*Downloading\s+\S+|added \d+ package|\[ERROR\]|BUILD (SUCCESS|FAILED)|\s*Finished\s+|Successfully (installed|built)|ERROR:)`)
	rTree=regexp.MustCompile("[├└]──|│  ")
	rGrep=regexp.MustCompile(`^[^:\n]+:\d+:`)
	rNumbered=regexp.MustCompile(`^\s*\d+\|`)
	rLSrow=regexp.MustCompile(`(?m)^[-dlbcps][rwx-]{9}`)
	rSearchHeader=regexp.MustCompile(`^Result of search in '[^']*' \(total \d+ files?\):`)
)
func detect(input string)string{
	head:=input;if len(head)>1024{head=head[:1024]}
	switch{case rGitLog.MatchString(head):return "git-log";case rGitDiff.MatchString(head)||rGitHunk.MatchString(head):return "git-diff";case rGitStatus.MatchString(head):return "git-status";case rBuild.MatchString(head):return "build-output";case mostlyPorcelain(head):return "git-status"}
	lines:=strings.Split(head,"
");nonempty:=[]string{};for _,l:=range lines{if strings.TrimSpace(l)!=""{nonempty=append(nonempty,l)}}
	for _,l:=range nonempty[:min(5,len(nonempty))]{if rGrep.MatchString(l){return "grep"}}
	allPaths:=len(nonempty)>=3;for _,l:=range nonempty{t:=strings.TrimSpace(l);if strings.Contains(t,":")||strings.ContainsAny(t," 	"){allPaths=false;break}};if allPaths{return "find"}
	if rTree.MatchString(head){return "tree"};if strings.Contains(head,"total ")||len(rLSrow.FindAllString(head,-1))>=3{return "ls"};if rSearchHeader.MatchString(head){return "search-list"}
	numbered:=0;for _,l:=range lines{if rNumbered.MatchString(l){numbered++}};if len(lines)>=250&&float64(numbered)/float64(len(lines))>=0.7{return "read-numbered"}
	if len(nonempty)>=5{return "dedup-log"};if len(strings.Split(input,"
"))>=250{return "smart-truncate"};return ""
}
func mostlyPorcelain(s string)bool{n,ok:=0,0;for _,l:=range strings.Split(s,"
"){if strings.TrimSpace(l)==""{continue};n++;if rPorcelain.MatchString(l){ok++}};return n>0&&float64(ok)/float64(n)>=.6}
func filterText(name,input string)string{switch name{case "git-diff":return gitDiff(input);case "git-status":return gitStatus(input);case "git-log":return gitLog(input);case "build-output":return buildOutput(input);case "grep":return grep(input);case "find":return find(input);case "tree":return tree(input);case "ls":return ls(input);case "search-list":return searchList(input);case "read-numbered":return truncateNumbered(input);case "dedup-log":return dedup(input);case "smart-truncate":return smartTruncate(input)};return input}
func gitDiff(input string)string{
	out:=[]string{};file:="";added,removed:=0,0;inHunk,shown:=false,0;skipped:=0;truncated:=false
	flush:=func(){if skipped>0{out=append(out,fmt.Sprintf("  ... (%d lines truncated)",skipped));truncated=true;skipped=0}}
	for _,line:=range strings.Split(input,"
"){if strings.HasPrefix(line,"diff --git "){flush();if file!=""&&(added>0||removed>0){out=append(out,fmt.Sprintf("  +%d -%d",added,removed))};parts:=strings.Split(line," b/");file="unknown";if len(parts)>1{file=strings.Join(parts[1:]," b/")};out=append(out,"
"+file);added,removed=0,0;inHunk=false;shown=0
	}else if strings.HasPrefix(line,"@@"){flush();inHunk=true;shown=0;out=append(out,"  "+line)
	}else if inHunk{switch{case strings.HasPrefix(line,"+")&&!strings.HasPrefix(line,"+++"):added++;if shown<100{out=append(out,"  "+line);shown++}else{skipped++}
	case strings.HasPrefix(line,"-")&&!strings.HasPrefix(line,"---"):removed++;if shown<100{out=append(out,"  "+line);shown++}else{skipped++}
	case shown>0&&shown<100&&!strings.HasPrefix(line,"\"):out=append(out,"  "+line);shown++}}
	if len(out)>=500{out=append(out,"
... (more changes truncated)");truncated=true;break}}
	flush();if file!=""&&(added>0||removed>0){out=append(out,fmt.Sprintf("  +%d -%d",added,removed))};if truncated{out=append(out,"[full diff: rtk git diff --no-compact]")};return strings.Join(out,"
")
}
func gitStatus(input string)string{
	branch:="";staged,modified,untracked:=[]string{},[]string{},[]string{};conflicts:=0
	for _,raw:=range strings.Split(input,"
"){t:=strings.TrimSpace(raw);if t==""{continue};if strings.HasPrefix(t,"On branch "){branch=strings.TrimPrefix(t,"On branch ");continue};if strings.HasPrefix(t,"##"){branch=strings.TrimSpace(strings.TrimPrefix(t,"##"));continue};if len(raw)>=3&&rPorcelain.MatchString(raw){x,y:=raw[0],raw[1];f:=raw[3:];if raw[:2]=="??"{untracked=append(untracked,f);continue};if strings.ContainsRune("MADRC",rune(x)){staged=append(staged,f)}else if x=='U'{conflicts++};if y=='M'||y=='D'{modified=append(modified,f)}}}
	out:="";if branch!=""{out+="* "+branch+"
"};write:=func(prefix,label string,list []string,cap int){if len(list)==0{return};out+=fmt.Sprintf("%s %s: %d files
",prefix,label,len(list));for _,f:=range list[:min(cap,len(list))]{out+="   "+f+"
"};if len(list)>cap{out+=fmt.Sprintf("   ... +%d more
",len(list)-cap)}};write("+","Staged",staged,10);write("~","Modified",modified,10);write("?","Untracked",untracked,10);if conflicts>0{out+=fmt.Sprintf("conflicts: %d files
",conflicts)};if len(staged)+len(modified)+len(untracked)+conflicts==0{out+="clean — nothing to commit
"};return strings.TrimRight(out,"
")
}
func gitLog(input string)string{lines:=strings.Split(input,"
");out:=[]string{};in,subject:=false,false;skipped:=0;push:=func(s string){if len(out)<200{out=append(out,s)}else{skipped++}};for _,raw:=range lines{line:=strings.TrimRight(raw," 	");t:=strings.TrimSpace(line);if regexp.MustCompile(`(?i)^commit [0-9a-f]{7,40}$`).MatchString(t)||regexp.MustCompile(`(?i)^[*|/\\ ]+commit [0-9a-f]{7,40}`).MatchString(t){in=true;subject=false;push(line);continue};if in{if strings.HasPrefix(t,"Author:")||strings.HasPrefix(t,"Date:"){push(t);continue};if t==""{continue};if !subject&&regexp.MustCompile(`^[*|/\\ ]*    \S`).MatchString(line){push("  Subject: "+t);subject=true;continue};if regexp.MustCompile(`^\d+ file\w* changed`).MatchString(t){push("  "+t);continue};if strings.HasPrefix(t,"diff --git "){push("  ... diff body omitted")};continue};push(t)};if skipped>0{out=append(out,fmt.Sprintf("... (%d more lines)",skipped))};result:=strings.Join(out,"
");if result==""||len(result)>len(input){return input};return result}
func grep(input string)string{type pair struct{n,t string};groups:=map[string][]pair{};total:=0;for _,line:=range strings.Split(input,"
"){i:=strings.Index(line,":");if i<0{continue};j:=strings.Index(line[i+1:],":");if j<0{continue};j+=i+1;n:=line[i+1:j];if !regexp.MustCompile(`^\d+$`).MatchString(n){continue};groups[line[:i]]=append(groups[line[:i]],pair{n,strings.TrimSpace(line[j+1:])});total++};if total==0{return input};files:=[]string{};for f:=range groups{files=append(files,f)};sort.Strings(files);out:=fmt.Sprintf("%d matches in %dF:

",total,len(files));for _,f:=range files{items:=groups[f];out+=fmt.Sprintf("[file] %s (%d):
",f,len(items));for _,item:=range items[:min(10,len(items))]{out+=fmt.Sprintf("  %4s: %s
",item.n,item.t)};if len(items)>10{out+=fmt.Sprintf("  +%d
",len(items)-10)};out+="
"};return out}
func find(input string)string{groups:=map[string][]string{};total:=0;for _,raw:=range strings.Split(input,"
"){p:=strings.TrimSpace(raw);if p==""{continue};i:=strings.LastIndexAny(p,"/\");dir,name:=".",p;if i>=0{dir=p[:i];name=p[i+1:];if dir==""{dir="/"}};groups[dir]=append(groups[dir],name);total++};dirs:=[]string{};for d:=range groups{dirs=append(dirs,d)};sort.Strings(dirs);out:=fmt.Sprintf("%d files in %d dirs:

",total,len(dirs));for _,dir:=range dirs[:min(20,len(dirs))]{items:=groups[dir];out+=fmt.Sprintf("%s/  (%d)
",strings.ReplaceAll(dir,"\","/"),len(items));for _,v:=range items[:min(10,len(items))]{out+="  "+v+"
"};if len(items)>10{out+=fmt.Sprintf("  +%d
",len(items)-10)}};if len(dirs)>20{out+=fmt.Sprintf("
+%d more dirs
",len(dirs)-20)};return out}
func tree(input string)string{out:=[]string{};for _,l:=range strings.Split(input,"
"){if strings.Contains(l,"director")&&strings.Contains(l,"file"){continue};if strings.TrimSpace(l)==""&&len(out)==0{continue};out=append(out,l)};for len(out)>0&&strings.TrimSpace(out[len(out)-1])==""{out=out[:len(out)-1]};if len(out)>200{return strings.Join(out[:200],"
")+fmt.Sprintf("
... +%d more lines",len(out)-200)};return strings.Join(out,"
")}
func dedup(input string)string{lines:=strings.Split(input,"
");out:=[]string{};prev:="";count:=0;blank:=false;flush:=func(){if prev!=""&&count>1{out=append(out,fmt.Sprintf("  ... (%d duplicate lines)",count-1))}};for _,l:=range lines{if strings.TrimSpace(l)==""{if !blank{out=append(out,l)};blank=true;flush();prev="";count=0;continue};blank=false;if l==prev{count++;continue};flush();out=append(out,l);prev=l;count=1;if len(out)>=2000{out=append(out,"... (truncated at 2000 lines)");return strings.Join(out,"
")}};flush();return strings.Join(out,"
")}
func smartTruncate(input string)string{lines:=strings.Split(input,"
");if len(lines)<250{return input};return strings.Join(append(append(append([]string{},lines[:120]...),fmt.Sprintf("... +%d lines truncated",len(lines)-180)),lines[len(lines)-60:]...),"
")}
func truncateNumbered(input string)string{lines:=strings.Split(input,"
");if len(lines)<250{return input};return strings.Join(append(append(append([]string{},lines[:120]...),fmt.Sprintf("... +%d lines truncated (file continues)",len(lines)-180)),lines[len(lines)-60:]...),"
")}
func searchList(input string)string{lines:=strings.Split(input,"
");if len(lines)<2{return input};paths:=[]string{};for _,line:=range lines[1:]{t:=strings.TrimSpace(line);if strings.HasPrefix(t,"- "){paths=append(paths,strings.TrimPrefix(t,"- "))}};if len(paths)==0{return input};groups:=map[string][]string{};for _,p:=range paths{i:=strings.LastIndex(p,"/");dir,name:=".",p;if i>=0{dir=p[:i];name=p[i+1:]};groups[dir]=append(groups[dir],name)};dirs:=[]string{};for d:=range groups{dirs=append(dirs,d)};sort.Strings(dirs);out:=fmt.Sprintf("%s
%d files in %d dirs:

",lines[0],len(paths),len(dirs));for _,d:=range dirs[:min(20,len(dirs))]{values:=groups[d];out+=fmt.Sprintf("%s/ (%d):
",d,len(values));for _,v:=range values[:min(10,len(values))]{out+="  "+v+"
"};if len(values)>10{out+=fmt.Sprintf("  +%d
",len(values)-10)};out+="
"};if len(dirs)>20{out+=fmt.Sprintf("+%d more dirs
",len(dirs)-20)};return strings.TrimRight(out,"
")}
func ls(input string)string{lines:=strings.Split(input,"
");dirs,files:=[]string{},[]string{};counts:=map[string]int{};date:=regexp.MustCompile(`\s+(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\s+\d{1,2}\s+(\d{4}|\d{2}:\d{2})\s+`);noise:=map[string]bool{};for _,n:=range strings.Fields("node_modules .git target __pycache__ .next dist build .cache .turbo .vercel .pytest_cache .mypy_cache .tox .venv venv env coverage .nyc_output .DS_Store Thumbs.db .idea .vscode .vs .egg-info .eggs"){noise[n]=true};for _,line:=range lines{if strings.HasPrefix(line,"total ")||line==""{continue};loc:=date.FindStringIndex(line);if loc==nil{continue};name:=line[loc[1]:];before:=strings.Fields(line[:loc[0]]);if len(before)<4{continue};typ:=before[0][0];size:=int64(0);for i:=len(before)-1;i>=0;i--{if n,err:=fmt.Sscanf(before[i],"%d",&size);err==nil&&n==1{break}};if name=="."||name==".."||noise[name]{continue};if typ=='d'{dirs=append(dirs,name)}else if typ=='-'||typ=='l'{ext:="no ext";if i:=strings.LastIndex(name,".");i>0{ext=name[i:]};counts[ext]++;sz:=fmt.Sprintf("%dB",size);if size>=1048576{sz=fmt.Sprintf("%.1fM",float64(size)/1048576)}else if size>=1024{sz=fmt.Sprintf("%.1fK",float64(size)/1024)};files=append(files,fmt.Sprintf("%s  %s",name,sz))}};if len(dirs)+len(files)==0{return input};out:="";for _,d:=range dirs{out+=d+"/
"};for _,f:=range files{out+=f+"
"};exts:=[]string{};for e:=range counts{exts=append(exts,e)};sort.Slice(exts,func(i,j int)bool{return counts[exts[i]]>counts[exts[j]]});out+=fmt.Sprintf("
Summary: %d files, %d dirs",len(files),len(dirs));if len(exts)>0{parts:=[]string{};for _,e:=range exts[:min(5,len(exts))]{parts=append(parts,fmt.Sprintf("%d %s",counts[e],e))};out+=" ("+strings.Join(parts,", ");if len(exts)>5{out+=fmt.Sprintf(", +%d more",len(exts)-5)};out+=")"};return out}
func buildOutput(input string)string{errorsFound,warnings,summary:=[]string{},[]string{},[]string{};compiling,downloading:=0,0;for _,line:=range strings.Split(input,"
"){t:=strings.TrimSpace(line);if t==""{continue};switch{case strings.HasPrefix(t,"npm ERR!")||strings.HasPrefix(t,"npm error")||strings.HasPrefix(t,"yarn error")||strings.HasPrefix(t,"ERROR:")||strings.HasPrefix(t,"[ERROR]")||strings.HasPrefix(t,"BUILD FAILED")||strings.HasPrefix(strings.ToLower(t),"error["):errorsFound=append(errorsFound,line);case strings.HasPrefix(strings.ToLower(t),"npm warn")||strings.HasPrefix(strings.ToLower(t),"yarn warn")||strings.HasPrefix(t,"[WARNING]")||strings.HasPrefix(strings.ToLower(t),"warning["):warnings=append(warnings,line);case strings.HasPrefix(t,"Compiling "):compiling++;case strings.HasPrefix(t,"Downloading ")||strings.HasPrefix(t,"Fetching "):downloading++;case strings.HasPrefix(t,"Finished ")||strings.HasPrefix(t,"BUILD SUCCESS")||strings.HasPrefix(t,"Successfully installed")||strings.HasPrefix(t,"Successfully built")||strings.HasPrefix(t,"added "):summary=append(summary,line)}};out:="";for _,v:=range warnings[:min(3,len(warnings))]{out+=v+"
"};if len(warnings)>3{out+=fmt.Sprintf("... +%d additional warnings
",len(warnings)-3)};if len(errorsFound)>0{out+=strings.Join(errorsFound,"
")+"
"};if len(summary)>0{out+=strings.Join(summary,"
")+"
"};if compiling>0{out+=fmt.Sprintf("... %d compiling lines omitted
",compiling)};if downloading>0{out+=fmt.Sprintf("... %d download lines omitted
",downloading)};return strings.TrimRight(out,"
")}
