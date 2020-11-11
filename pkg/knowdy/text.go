package knowdy

import (
	"bytes"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"time"
)

func (s *Shard) DecodeText(text string, lang string) (string, string, error) {
	u := url.URL{Scheme: "http", Host: s.LingProcAddress, Path: "/decode"}
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
		return "", "", err
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)

	dt := resp.Header.Get("GLT-Discourse-Type")
	// if dt != "" {
	//	log.Println("discourse type: ", dt)
	// }
	return string(body), dt, nil
}

func (s *Shard) EncodeText(graph string, lang string) (string, error) {
	u := url.URL{Scheme: "http", Host: s.LingProcAddress, Path: "/encode"}
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


/*
pass phrase generation

Please note that it's an auto-generated list of randomized phrases.

As such
any  offensive or otherwise inappropriate phrase
  please mark it so that we exclude it from further suggestions to anybody
Thank you for understanding and cooperation.


*/
