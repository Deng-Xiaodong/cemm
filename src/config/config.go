package config

type Config struct {
	ClientCnt int `json:"client_cnt"`
	Volume    int `json:"volume"`
	Limit     int `json:"limit"`
}

var defaultConfig = Config{
	ClientCnt: 4,
	//Volume:    1 << 10,
	//Limit:     1 << 24,
	Volume: 1_000,
	Limit:  1_000_000_000,
}

func GetDefaultConfig() *Config {
	return &defaultConfig
}
