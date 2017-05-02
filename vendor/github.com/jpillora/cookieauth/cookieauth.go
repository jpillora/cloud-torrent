package cookieauth

import (
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	scrypt "github.com/elithrar/simple-scrypt"
	lru "github.com/hashicorp/golang-lru"
)

const (
	cacheSize = 1024
	fortnight = 14 * 24 * time.Hour
)

var params = scrypt.Params{N: 16384, R: 8, P: 1, SaltLen: 16, DKLen: 32}

func New() *CookieAuth {
	l, _ := lru.New(cacheSize)
	ca := &CookieAuth{
		id:     "cookieauth",
		expiry: fortnight,
		auth:   nil,
		logger: nil,
		cache:  l,
		next:   nil,
	}
	return ca
}

type CookieAuth struct {
	mut    sync.RWMutex
	id     string
	auth   []byte
	expiry time.Duration
	logger *log.Logger
	cache  *lru.Cache
	next   http.Handler
}

func Wrap(next http.Handler, user, pass string) http.Handler {
	ca := New()
	ca.SetUserPass(user, pass)
	return ca.Wrap(next)
}

func (ca *CookieAuth) Wrap(next http.Handler) http.Handler {
	ca.SetNextHandler(next)
	return ca
}

//SetNextHandler sets the next http.Handler to use in
//this middle chain
func (ca *CookieAuth) SetNextHandler(next http.Handler) {
	ca.mut.Lock()
	ca.next = next
	ca.mut.Unlock()
}

//SetUserPass sets the current username and password
func (ca *CookieAuth) SetUserPass(user, pass string) {
	ca.mut.Lock()
	if user == "" && pass == "" {
		ca.auth = nil
	} else {
		ca.auth = concat(user, pass)
	}
	ca.cache.Purge()
	ca.mut.Unlock()
}

//SetExpiry defines the cookie expiration time
func (ca *CookieAuth) SetExpiry(expiry time.Duration) {
	ca.mut.Lock()
	ca.expiry = expiry
	ca.mut.Unlock()
}

//SetLogger sets the log.Logger used to
//display actions
func (ca *CookieAuth) SetLogger(l *log.Logger) {
	ca.mut.Lock()
	ca.logger = l
	ca.mut.Unlock()
}

//SetID changes the cookie ID, defaults
//to "cookieauth"
func (ca *CookieAuth) SetID(id string) {
	ca.mut.Lock()
	ca.id = id
	ca.mut.Unlock()
}

func (ca *CookieAuth) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ca.mut.RLock()
	caid := ca.id
	ca.mut.RUnlock()
	//no creds
	if len(ca.getAuth()) == 0 {
		ca.next.ServeHTTP(w, r)
		return
	}
	//login with creds
	if u, p, ok := r.BasicAuth(); ok {
		b64, err := ca.authWithCreds(u, p)
		if err != nil {
			ca.logf("login error: %s", err)
			ca.authFailed(w)
			return
		}
		//set cookie
		http.SetCookie(w, &http.Cookie{
			Name:    caid,
			Value:   b64,
			Expires: time.Now().Add(ca.expiry),
		})
		ca.logf("login success")
		ca.next.ServeHTTP(w, r)
		return
	}
	//login with token
	for _, c := range r.Cookies() {
		if c.Name == caid {
			b64 := c.Value
			if err := ca.authWithToken(b64); err != nil {
				ca.logf("token error:  %s", err)
				ca.authFailed(w)
				return
			}
			ca.mut.RLock()
			expires := time.Now().Add(ca.expiry)
			ca.mut.RUnlock()
			http.SetCookie(w, &http.Cookie{
				Name:    ca.id,
				Value:   b64,
				Expires: expires,
			})
			ca.logf("token success")
			ca.next.ServeHTTP(w, r)
			return
		}
	}
	//no auth detected!
	ca.logf("not authenticated")
	ca.authFailed(w)
}

func (ca *CookieAuth) getAuth() []byte {
	ca.mut.RLock()
	b := make([]byte, len(ca.auth))
	copy(b, ca.auth)
	ca.mut.RUnlock()
	return b
}

func (ca *CookieAuth) authFailed(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: ca.id, MaxAge: -1})
	w.Header().Set("WWW-Authenticate", "Basic")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(http.StatusText(http.StatusUnauthorized)))
}

func (ca *CookieAuth) authWithCreds(user, pass string) (string, error) {
	//check password
	if subtle.ConstantTimeCompare(ca.auth, concat(user, pass)) != 1 {
		return "", errors.New("incorrect password")
	}
	//cached token?
	if b64, ok := ca.cache.Get(string(ca.auth)); ok {
		return b64.(string), nil
	}
	//generate password hash
	hash, err := scrypt.GenerateFromPassword(ca.auth, params)
	if err != nil {
		return "", errors.New("hash failed")
	}
	//encode base64
	b64 := base64.StdEncoding.EncodeToString(hash)
	ca.cache.Add(string(ca.auth), b64)
	return b64, nil
}

func (ca *CookieAuth) authWithToken(b64 string) error {
	//cached token?
	if ca.cache.Contains(b64) {
		return nil
	}
	//decode base64
	hash, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return errors.New("b64 error")
	}
	//check password hash
	if err := scrypt.CompareHashAndPassword(hash, ca.getAuth()); err != nil {
		return err
	}
	//cache success
	ca.cache.Add(b64, true)
	return nil
}

func (ca *CookieAuth) logf(format string, args ...interface{}) {
	ca.mut.RLock()
	l := ca.logger
	ca.mut.RUnlock()
	if l != nil {
		l.Printf("[cookieauth] "+format, args...)
	}
}

func concat(u, p string) []byte {
	return []byte(u + ":" + p)
}
