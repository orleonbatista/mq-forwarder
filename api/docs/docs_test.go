package docs_test

import (
	"testing"

	"mq-transfer-go/api/docs"
)

func TestDocsInit(t *testing.T) {
	if docs.OpenAPIInfo == nil {
		t.Fatal("OpenAPIInfo should not be nil")
	}
}
