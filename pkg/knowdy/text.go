package knowdy

import (
        "fmt"
	"io/ioutil"
        "net/http"
        "log"
)

func DecodeText(text string, addr string) (string, error) {
	log.Println("TEXT:", text)
	log.Println("GLT:", addr)
        url := "http://" + addr + "/text-to-graph?t=" + text
        fmt.Println("URL:", url)

        resp, err := http.Get(url)
        if err != nil {
                 panic(err)
        }
        defer resp.Body.Close()

        fmt.Println("response Status:", resp.Status)
        fmt.Println("response Headers:", resp.Header)
        body, _ := ioutil.ReadAll(resp.Body)
        fmt.Println("response Body:", string(body))

        return "OK", nil
}

func EncodeText(graph string, addr string) (string, error) {
	log.Println("GRAPH:", graph)
	log.Println("GLT:", addr)
        return "OK", nil
}