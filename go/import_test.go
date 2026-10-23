package bbmask_test

import (
	"testing"

	"github.com/bigbrains/bb-mask-sdk/go"
)

func TestDefaultImportName(t *testing.T) {
	if bbmask.DefaultBaseURL == "" || bbmask.Version == "" {
		t.Fatal("paket adı bbmask olmalı")
	}
}
