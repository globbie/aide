package knowdy

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
)

func DecodeText(text string, addr string) (string, error) {
	Url, err := url.Parse("http://" + addr)
	if err != nil {
		panic("invalid URL")
	}
	Url.Path = "/text-to-graph"
	parameters := url.Values{}
	parameters.Add("t", text)
	Url.RawQuery = parameters.Encode()

	fmt.Printf("Encoded URL is %q\n", Url.String())

	resp, err := http.Get(Url.String())
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	fmt.Println("response Status:", resp.Status)
	fmt.Println("response Headers:", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println("response Body:", string(body))

	return string(body), nil
}

func EncodeText(graph string, codeSystem string, addr string) (string, error) {
	Url, err := url.Parse("http://" + addr)
	if err != nil {
		panic("invalid URL")
	}
	Url.Path = "/graph-to-text"
	parameters := url.Values{}
	parameters.Add("cs", codeSystem)
	Url.RawQuery = parameters.Encode()

	fmt.Printf("== URL is %q  Graph:%q\n", Url.String(), graph)

	resp, err := http.Post(Url.String(), "text/plain; charset=utf-8", bytes.NewBuffer([]byte(graph)))
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	fmt.Println("response Status:", resp.Status)
	fmt.Println("response Headers:", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)

	return string(body), nil
}
