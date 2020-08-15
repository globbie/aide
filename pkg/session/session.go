package session

import (
	"fmt"
	"net"
	"net/http"
	"strings"

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
	Threads     []ChatThread
}

func New(r *http.Request) (*ChatSession, error) {
	cs := ChatSession{
		UserAgent: r.UserAgent(),
	}
	ip, err := getSessionIP(r)
	if err == nil {
		cs.UserIP = ip
	}
	cs.Langs, _, _ = language.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
	
	return &cs, nil
}

func buildSessionCookie(name string, val string, domain string) (*http.Cookie, error) {
	cookie := http.Cookie{}
	cookie.Name = name
	cookie.Value = val
	cookie.Path = "/"
	cookie.Domain = domain
	cookie.Expires = time.Now().Add(356 * 24 * time.Hour)
	cookie.HttpOnly = true
	return &cookie, nil
}

func getSessionIP(r *http.Request) (string, error) {
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
    return "", fmt.Errorf("No valid IP found")
}
