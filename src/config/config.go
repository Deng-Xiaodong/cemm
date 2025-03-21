package config

import "math"

type Config struct {
	ClientCnt int `json:"client_cnt"`
	Volume    int `json:"volume"`
	Limit     int `json:"limit"`
}

var defaultConfig = Config{
	ClientCnt: 4,
	Volume:    1000,
	Limit:     math.MaxInt64,
}

func GetDefaultConfig() *Config {
	return &defaultConfig
}
