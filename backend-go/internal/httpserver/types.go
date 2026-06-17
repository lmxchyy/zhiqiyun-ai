package httpserver

type platformData struct {
	GenerationTasks []generationTask `json:"generationTasks"`
	Assets          []asset          `json:"assets"`
	Counters        map[string]int   `json:"counters"`
}

type generationTask struct {
	ID               string         `json:"id"`
	UserID           string         `json:"userId"`
	Type             string         `json:"type"`
	Prompt           string         `json:"prompt"`
	Params           map[string]any `json:"params"`
	Model            string         `json:"model"`
	Status           string         `json:"status"`
	Progress         int            `json:"progress"`
	PointCost        int            `json:"pointCost"`
	ResultIDs        []string       `json:"resultIds"`
	Error            any            `json:"error"`
	CreatedAt        string         `json:"createdAt"`
	UpdatedAt        string         `json:"updatedAt"`
	WorkerFinishedAt string         `json:"workerFinishedAt,omitempty"`
}

type asset struct {
	ID        string         `json:"id"`
	UserID    string         `json:"userId"`
	TaskID    string         `json:"taskId"`
	Name      string         `json:"name"`
	MediaType string         `json:"mediaType"`
	URL       string         `json:"url"`
	Favorite  bool           `json:"favorite"`
	Metadata  map[string]any `json:"metadata"`
	CreatedAt string         `json:"createdAt"`
	UpdatedAt string         `json:"updatedAt"`
}

type createGenerationTaskRequest struct {
	Type   string         `json:"type"`
	Prompt string         `json:"prompt"`
	Model  string         `json:"model"`
	Params map[string]any `json:"params"`
}
