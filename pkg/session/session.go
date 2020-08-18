package session

import (
	"errors"
	"crypto/rsa"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go"
	"golang.org/x/text/language"
)

type ChatThread struct {
	ThreadID     string
}

type ChatSession struct {
	UserID      string
	ShardID     string
	UserAgent   string
	UserIP      string
	Langs       []language.Tag
	Roles       []string
	Threads     []ChatThread
}

type Claims struct {
	*jwt.StandardClaims
	UserID    string   `json:"userid,required"`
	ShardID   string   `json:"shardid,required"`
	UserRoles []string `json:"roles,omitempty"`
}

func New(r *http.Request) (*ChatSession, error) {
	cs := ChatSession{
		UserAgent: r.UserAgent(),
	}
	ip, err := GetSessionIP(r)
	if err == nil {
		cs.UserIP = ip
	}
	cs.Langs, _, _ = language.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
	
	return &cs, nil
}

func BuildSessionCookie(name string, val string, domain string) (*http.Cookie, error) {
	cookie := http.Cookie{}
	cookie.Name = name
	cookie.Value = val
	cookie.Path = "/"
	if domain != "localhost" {
		cookie.Domain = domain
	}
	cookie.Expires = time.Now().Add(356 * 24 * time.Hour)
	cookie.HttpOnly = true
	return &cookie, nil
}

func GetSessionIP(r *http.Request) (string, error) {
    ip := r.Header.Get("X-REAL-IP")
    netIP := net.ParseIP(ip)
    if netIP != nil {
        return ip, nil
    }
    ips := r.Header.Get("X-FORWARDED-FOR")
    splitIps := strings.Split(ips, ",")
    for _, ip := range splitIps {
        netIP := net.ParseIP(ip)
        if netIP != nil {
            return ip, nil
        }
    }
    ip, _, err := net.SplitHostPort(r.RemoteAddr)
    if err != nil {
        return "", err
    }
    netIP = net.ParseIP(ip)
    if netIP != nil {
        return ip, nil
    }
    return "", errors.New("No valid IP found")
}

func IssueAccessToken(ses *ChatSession, signKey *rsa.PrivateKey, expiry int) (string, error) {
	token := jwt.New(jwt.GetSigningMethod("RS256"))
	token.Claims = &Claims{
		StandardClaims: &jwt.StandardClaims{
			ExpiresAt: time.Now().Add(time.Hour * time.Duration(expiry)).Unix(),
		},
		ShardID: ses.ShardID,
		UserID: ses.UserID,
		UserRoles: ses.Roles,
	}
	return token.SignedString(signKey)
}
