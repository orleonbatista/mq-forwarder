package docs_test

import (
	"testing"

	"mq-forwarder-go/api/docs"
)

func TestDocsInit(t *testing.T) {
	if docs.OpenAPIInfo == nil {
		t.Fatal("OpenAPIInfo should not be nil")
	}
}
