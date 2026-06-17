package config

import "os"

type Config struct {
	Addr      string
	DataPath  string
	StaticDir string
}

func Load() Config {
	addr := os.Getenv("XIANZHI_GO_ADDR")
	if addr == "" {
		addr = os.Getenv("PORT")
	}
	if addr == "" {
		addr = "3100"
	}
	if addr[0] != ':' {
		addr = ":" + addr
	}
	dataPath := os.Getenv("XIANZHI_DATA_PATH")
	if dataPath == "" {
		dataPath = "data/store.json"
	}
	staticDir := os.Getenv("XIANZHI_STATIC_DIR")
	if staticDir == "" {
		staticDir = "frontend-vue/dist"
	}
	return Config{
		Addr:      addr,
		DataPath:  dataPath,
		StaticDir: staticDir,
	}
}
