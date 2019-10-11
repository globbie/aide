package knowdy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDecodeTextTimeout(t *testing.T) {
	shard := Shard{
		shard:      nil,
		gltAddress: "localhost",
		workers:    nil,
	}
	graph, err := shard.DecodeText("banana", "EN SyNode CS")
	if err == nil {
		t.Error(err)
	}
}

func TestDecodeTextSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "{class Banana}")
	}))
	defer ts.Close()

	shard := Shard{
		shard:      nil,
		gltAddress: ts.URL[7:], // strip off http:// prefix
		workers:    nil,
	}

	graph, err := shard.DecodeText("banana", "EN SyNode CS")
	if err != nil {
		t.Error(err)
	}

	// check the response body is what we expect
	expected := "{class Banana}"
	if graph != expected {
		t.Errorf("handler returned unexpected body: got %v want %v",
			graph, expected)
	}
}


