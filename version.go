package main

import (
	_ "embed"
	"encoding/json"

	"github.com/Oranjekop/TextViewer/internal/releaseinfo"
)

//go:embed mygo.json
var applicationConfig []byte

func applicationVersion() string {
	var config struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(applicationConfig, &config); err != nil {
		panic(err)
	}
	if _, err := releaseinfo.ParseVersion(config.Version); err != nil {
		panic(err)
	}
	return config.Version
}
