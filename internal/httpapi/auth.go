package httpapi

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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

const (
	loginAttemptThreshold = 5
	loginLockDuration = 15 * time.Minute
	loginEntryIdleTTL = 30 * time.Minute
	loginCleanupInterval = time.Minute
	maxLoginLimitEntries = 4096
)

type loginAttempt struct {
	Count int
	LockedUntil time.Time
	LastSeen time.Time
}

type loginLimiter struct {
	sync.Mutex
	entries map[string]loginAttempt
	lastCleanup time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{entries: make(map[string]loginAttempt)}
}

var loginLimit = newLoginLimiter()

// cleanupLocked removes old, unlocked entries at most once per minute.
func (l *loginLimiter) cleanupLocked(now time.Time) {
	if !l.lastCleanup.IsZero() && now.Sub(l.lastCleanup) < loginCleanupInterval {
		return
	}
	for ip, attempt := range l.entries {
		if !now.Before(attempt.LockedUntil) && now.Sub(attempt.LastSeen) >= loginEntryIdleTTL {
			delete(l.entries, ip)
		}
	}
	l.lastCleanup = now
}

// makeRoomLocked preserves active lockouts. It evicts the least-recently-seen
// unlocked entry rather than allowing attacker-controlled IP cardinality to
// grow the process indefinitely.
func (l *loginLimiter) makeRoomLocked(now time.Time) bool {
	if len(l.entries) < maxLoginLimitEntries {
		return true
	}
	oldestIP := ""
	var oldestSeen time.Time
	for ip, attempt := range l.entries {
		if now.Before(attempt.LockedUntil) {
			continue
		}
		if oldestIP == "" || attempt.LastSeen.Before(oldestSeen) {
			oldestIP = ip
			oldestSeen = attempt.LastSeen
		}
	}
	if oldestIP == "" {
		return false
	}
	delete(l.entries, oldestIP)
	return true
}

func (l *loginLimiter) nextUnlockDelayLocked(now time.Time) time.Duration {
	var soonest time.Duration
	for _, attempt := range l.entries {
		if now.Before(attempt.LockedUntil) {
			remaining := attempt.LockedUntil.Sub(now)
			if soonest == 0 || remaining < soonest {
				soonest = remaining
			}
		}
	}
	if soonest == 0 {
		return loginCleanupInterval
	}
	return soonest
}

func (l *loginLimiter) allow(ip string, now time.Time) (bool, time.Duration) {
	l.Lock()
	defer l.Unlock()
	l.cleanupLocked(now)
	if attempt, exists := l.entries[ip]; exists {
		attempt.LastSeen = now
		l.entries[ip] = attempt
		if now.Before(attempt.LockedUntil) {
			return false, attempt.LockedUntil.Sub(now)
		}
		return true, 0
	}
	if !l.makeRoomLocked(now) {
		return false, l.nextUnlockDelayLocked(now)
	}
	l.entries[ip] = loginAttempt{LastSeen: now}
	return true, 0
}

func (l *loginLimiter) recordFailure(ip string, now time.Time) {
	l.Lock()
	defer l.Unlock()
	l.cleanupLocked(now)
	attempt, exists := l.entries[ip]
	if !exists {
		if !l.makeRoomLocked(now) {
			return
		}
		attempt = loginAttempt{}
	}
	attempt.Count++
	attempt.LastSeen = now
	if attempt.Count >= loginAttemptThreshold {
		attempt.Count = 0
		attempt.LockedUntil = now.Add(loginLockDuration)
	}
	l.entries[ip] = attempt
}

func (l *loginLimiter) clear(ip string) {
	l.Lock()
	delete(l.entries, ip)
	l.Unlock()
}

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
	ip := clientIP(r)
	if allowed, retry := loginLimit.allow(ip, time.Now()); !allowed {
		wait := int(retry.Seconds()) + 1
		w.Header().Set("Retry-After", fmt.Sprint(wait))
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "Too many failed attempts", "retryAfter": wait})
		return
	}
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
		loginLimit.recordFailure(ip, time.Now())
		writeJSON(w,401,map[string]string{"error":"Invalid password"});return
	}
	loginLimit.clear(ip)
	if storedHash==""&&os.Getenv("INITIAL_PASSWORD")==""&&!isLocalRequest(r) {
		writeJSON(w,403,map[string]any{"success":false,"mustChangePassword":true,"error":"Default password must be changed from localhost before remote access, or set INITIAL_PASSWORD before startup"});return
	}
	if err:=a.setSessionCookie(w,r);err!=nil{writeJSON(w,500,map[string]string{"error":"Could not create session"});return}
	writeJSON(w,200,map[string]any{"success":true,"mustChangePassword":storedHash==""&&os.Getenv("INITIAL_PASSWORD")==""})
}
func (a *API) authLogout(w http.ResponseWriter,r *http.Request){clearSessionCookie(w,r);writeJSON(w,200,map[string]any{"success":true})}
