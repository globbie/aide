package knowdy

import (
	"bytes"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"time"
)

func (s *Shard) DecodeText(text string, lang string) (string, error) {
	u := url.URL{Scheme: "http", Host: s.LingProcAddress, Path: "/text-to-graph"}
	parameters := url.Values{}
	parameters.Add("t", text)
	parameters.Add("lang", lang)
	u.RawQuery = parameters.Encode()

	var netClient = &http.Client{
		Timeout: time.Second * 7,
	}

	resp, err := netClient.Get(u.String())
	if err != nil {
		log.Println(err.Error())
		return "", err
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	return string(body), nil
}

func (s *Shard) EncodeText(graph string, lang string) (string, error) {
	u := url.URL{Scheme: "http", Host: s.LingProcAddress, Path: "/graph-to-text"}
	parameters := url.Values{}
	parameters.Add("cs", lang)
	u.RawQuery = parameters.Encode()

	var netClient = &http.Client{
		Timeout: time.Second * 10,
	}

	resp, err := netClient.Post(u.String(), "text/plain; charset=utf-8", bytes.NewBuffer([]byte(graph)))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	return string(body), nil
}
