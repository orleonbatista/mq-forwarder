package docs_test

import (
	"testing"

	"mq-transfer-go/api/docs"
)

func TestDocsInit(t *testing.T) {
	if docs.SwaggerInfo == nil {
		t.Fatal("SwaggerInfo should not be nil")
	}
}
