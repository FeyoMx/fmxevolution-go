package instance

type EventConnectorConfig struct {
	ID      string   `json:"id,omitempty"`
	Enabled bool     `json:"enabled"`
	Events  []string `json:"events"`
}

type eventConnectorPayload struct {
	Enabled *bool    `json:"enabled"`
	Events  []string `json:"events"`
}

type EventConnectorUpdateEnvelope struct {
	Enabled   *bool                  `json:"enabled"`
	Events    []string               `json:"events"`
	Websocket *eventConnectorPayload `json:"websocket,omitempty"`
	Rabbitmq  *eventConnectorPayload `json:"rabbitmq,omitempty"`
	SQS       *eventConnectorPayload `json:"sqs,omitempty"`
}

type ProxyConfig struct {
	ID       string `json:"id,omitempty"`
	Enabled  bool   `json:"enabled"`
	Host     string `json:"host"`
	Port     string `json:"port"`
	Protocol string `json:"protocol"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type ChatSearchRequest struct {
	Where map[string]any `json:"where"`
}

type PartialFeatureResponse struct {
	Feature      string   `json:"feature"`
	Status       string   `json:"status"`
	Implemented  bool     `json:"implemented"`
	Message      string   `json:"message"`
	InstanceID   string   `json:"instance_id,omitempty"`
	InstanceName string   `json:"instanceName,omitempty"`
	BlockedBy    []string `json:"blocked_by,omitempty"`
}
