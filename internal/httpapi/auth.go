package httpapi

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const dashboardCookieName = "blackhand_session"
const sessionTTL = 12 * time.Hour

type loginAttempt struct { Count int; LockedUntil time.Time }
var loginLimit = struct{sync.Mutex; entries map[string]loginAttempt}{entries:map[string]loginAttempt{}}

func clientIP(r *http.Request) string {
	host,_,err:=net.SplitHostPort(r.RemoteAddr)
	if err==nil{return host}
	return r.RemoteAddr
}
func isLocalRequest(r *http.Request) bool {
	host:=strings.ToLower(strings.Split(r.Host,":")[0])
	if host=="localhost"||host=="127.0.0.1"||host=="::1"{return true}
	ip:=net.ParseIP(clientIP(r))
	return ip!=nil&&ip.IsLoopback()
}
func (a *API) dashboardSecret()([]byte,error){return a.store.PersistentSecret("dashboard-secret",32)}
func (a *API) sessionToken() (string,error) {
	secret,err:=a.dashboardSecret();if err!=nil{return "",err}
	nonce:=make([]byte,16);if _,err:=rand.Read(nonce);err!=nil{return "",err}
	payload:=fmt.Sprintf("%d:%s",time.Now().Add(sessionTTL).Unix(),hex.EncodeToString(nonce))
	mac:=hmac.New(sha256.New,secret);_,_=mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload))+"."+base64.RawURLEncoding.EncodeToString(mac.Sum(nil)),nil
}
func (a *API) validSession(r *http.Request) bool {
	cookie,err:=r.Cookie(dashboardCookieName);if err!=nil{return false}
	parts:=strings.Split(cookie.Value,".");if len(parts)!=2{return false}
	payload,err:=base64.RawURLEncoding.DecodeString(parts[0]);if err!=nil{return false}
	sig,err:=base64.RawURLEncoding.DecodeString(parts[1]);if err!=nil{return false}
	secret,err:=a.dashboardSecret();if err!=nil{return false}
	mac:=hmac.New(sha256.New,secret);_,_=mac.Write(payload)
	if !hmac.Equal(sig,mac.Sum(nil)){return false}
	fields:=strings.SplitN(string(payload),":",2);if len(fields)!=2{return false}
	var expires int64
	if _,err:=fmt.Sscan(fields[0],&expires);err!=nil{return false}
	return time.Now().Unix()<expires
}
func (a *API) setSessionCookie(w http.ResponseWriter,r *http.Request) error {
	token,err:=a.sessionToken();if err!=nil{return err}
	secure:=r.TLS!=nil||strings.EqualFold(r.Header.Get("X-Forwarded-Proto"),"https")
	http.SetCookie(w,&http.Cookie{Name:dashboardCookieName,Value:token,Path:"/",HttpOnly:true,Secure:secure,SameSite:http.SameSiteLaxMode,Expires:time.Now().Add(sessionTTL),MaxAge:int(sessionTTL.Seconds())})
	return nil
}
func clearSessionCookie(w http.ResponseWriter,r *http.Request) {
	secure:=r.TLS!=nil||strings.EqualFold(r.Header.Get("X-Forwarded-Proto"),"https")
	http.SetCookie(w,&http.Cookie{Name:dashboardCookieName,Value:"",Path:"/",HttpOnly:true,Secure:secure,SameSite:http.SameSiteLaxMode,Expires:time.Unix(0,0),MaxAge:-1})
}
func (a *API) dashboardAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		settings,err:=a.store.GetSettings(r.Context())
		if err!=nil {writeJSON(w,500,map[string]string{"error":"settings unavailable"});return}
		if !enabledSetting(settings["requireLogin"],true)||a.validSession(r){next.ServeHTTP(w,r);return}
		writeJSON(w,http.StatusUnauthorized,map[string]any{"error":"Dashboard login required","loginRequired":true})
	})
}
func (a *API) authSession(w http.ResponseWriter,r *http.Request) {
	settings,err:=a.store.GetSettings(r.Context())
	if err!=nil{writeJSON(w,500,map[string]string{"error":"settings unavailable"});return}
	_,hasPassword:=settings["password"]
	required:=enabledSetting(settings["requireLogin"],true)
	writeJSON(w,200,map[string]any{
		"authenticated":!required||a.validSession(r),
		"loginRequired":required,
		"hasPassword":hasPassword,
		"authMode":settings["authMode"],
		"initialPasswordConfigured":strings.TrimSpace(os.Getenv("INITIAL_PASSWORD"))!="",
	})
}
func enabledSetting(v any,fallback bool)bool{if v==nil{return fallback};b,ok:=v.(bool);if ok{return b};return fallback}
func (a *API) authLogin(w http.ResponseWriter,r *http.Request) {
	ip:=clientIP(r);loginLimit.Lock();attempt:=loginLimit.entries[ip]
	if time.Now().Before(attempt.LockedUntil){wait:=int(time.Until(attempt.LockedUntil).Seconds())+1;loginLimit.Unlock();w.Header().Set("Retry-After",fmt.Sprint(wait));writeJSON(w,429,map[string]any{"error":"Too many failed attempts","retryAfter":wait});return};loginLimit.Unlock()
	var body struct{Password string `json:"password"`}
	if err:=decodeJSON(w,r,&body,64<<10);err!=nil{writeJSON(w,400,map[string]string{"error":"Invalid JSON"});return}
	settings,err:=a.store.GetSettings(r.Context());if err!=nil{writeJSON(w,500,map[string]string{"error":"settings unavailable"});return}
	authMode,_:=settings["authMode"].(string)
	if authMode=="oidc"||authMode=="saml"||authMode=="sso" {
		oidcIssuer,_:=settings["oidcIssuerUrl"].(string);oidcClient,_:=settings["oidcClientId"].(string);oidcSecret,_:=settings["oidcClientSecret"].(string)
		samlEntry,_:=settings["samlEntryPoint"].(string);samlCert,_:=settings["samlCert"].(string)
		if (authMode=="oidc"||authMode=="sso")&&oidcIssuer!=""&&oidcClient!=""&&oidcSecret!="" {writeJSON(w,403,map[string]string{"error":"OIDC is configured but is not implemented in the Blackhand port"});return}
		if (authMode=="saml"||authMode=="sso")&&samlEntry!=""&&samlCert!="" {writeJSON(w,403,map[string]string{"error":"SAML is configured but is not implemented in the Blackhand port"});return}
	}
	storedHash,_:=settings["password"].(string)
	valid:=false
	if storedHash!="" {
		valid=bcrypt.CompareHashAndPassword([]byte(storedHash),[]byte(body.Password))==nil
	} else {
		initial:=os.Getenv("INITIAL_PASSWORD");if initial==""{initial="123456"};valid=body.Password==initial
	}
	if !valid {
		loginLimit.Lock();attempt=loginLimit.entries[ip];attempt.Count++;if attempt.Count>=5{attempt.LockedUntil=time.Now().Add(15*time.Minute);attempt.Count=0};loginLimit.entries[ip]=attempt;loginLimit.Unlock()
		writeJSON(w,401,map[string]string{"error":"Invalid password"});return
	}
	loginLimit.Lock();delete(loginLimit.entries,ip);loginLimit.Unlock()
	if storedHash==""&&os.Getenv("INITIAL_PASSWORD")==""&&!isLocalRequest(r) {
		writeJSON(w,403,map[string]any{"success":false,"mustChangePassword":true,"error":"Default password must be changed from localhost before remote access, or set INITIAL_PASSWORD before startup"});return
	}
	if err:=a.setSessionCookie(w,r);err!=nil{writeJSON(w,500,map[string]string{"error":"Could not create session"});return}
	writeJSON(w,200,map[string]any{"success":true,"mustChangePassword":storedHash==""&&os.Getenv("INITIAL_PASSWORD")==""})
}
func (a *API) authLogout(w http.ResponseWriter,r *http.Request){clearSessionCookie(w,r);writeJSON(w,200,map[string]any{"success":true})}
var _ = errors.Is
