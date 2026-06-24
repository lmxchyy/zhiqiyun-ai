package config

import "os"

type Config struct {
	Addr                string
	DataPath            string
	StaticDir           string
	AdminStaticDir      string
	DatabaseURL         string
	RedisURL            string
	ModelProviderURL    string
	ModelProviderAPIKey string
	ImageModel          string
	ModelTimeoutMS      string
	ModelProvidersJSON  string
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
	adminStaticDir := os.Getenv("XIANZHI_ADMIN_STATIC_DIR")
	if adminStaticDir == "" {
		adminStaticDir = "admin-vue/dist"
	}
	modelProviderURL := os.Getenv("MODEL_PROVIDER_URL")
	if modelProviderURL == "" {
		modelProviderURL = os.Getenv("OPENAI_BASE_URL")
	}
	modelProviderAPIKey := os.Getenv("MODEL_PROVIDER_API_KEY")
	if modelProviderAPIKey == "" {
		modelProviderAPIKey = os.Getenv("OPENAI_API_KEY")
	}
	imageModel := os.Getenv("MODEL_PROVIDER_IMAGE_MODEL")
	if imageModel == "" {
		imageModel = "gpt-image-2"
	}
	modelProvidersJSON := os.Getenv("MODEL_PROVIDERS_JSON")
	modelTimeoutMS := os.Getenv("MODEL_PROVIDER_TIMEOUT_MS")
	if modelTimeoutMS == "" {
		modelTimeoutMS = "30000"
	}
	return Config{
		Addr:                addr,
		DataPath:            dataPath,
		StaticDir:           staticDir,
		AdminStaticDir:      adminStaticDir,
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		RedisURL:            os.Getenv("REDIS_URL"),
		ModelProviderURL:    modelProviderURL,
		ModelProviderAPIKey: modelProviderAPIKey,
		ImageModel:          imageModel,
		ModelTimeoutMS:      modelTimeoutMS,
		ModelProvidersJSON:  modelProvidersJSON,
	}
}
