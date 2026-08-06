package v1

type notifyBarkConfig struct {
	Enabled       bool   `json:"enabled"`
	Server        string `json:"server"`
	Key           string `json:"key"` // 脱敏
	KeyConfigured bool   `json:"key_configured"`
}
type notifyTGConfig struct {
	Enabled         bool   `json:"enabled"`
	ChatID          string `json:"chat_id"`
	APIURL          string `json:"api_url"`
	Token           string `json:"token"` // 脱敏
	TokenConfigured bool   `json:"token_configured"`
}
type notifyQQConfig struct {
	Enabled             bool   `json:"enabled"`
	AppID               string `json:"app_id"`
	OpenID              string `json:"openid"`
	GroupOpenID         string `json:"group_openid"`
	AppSecret           string `json:"app_secret"` // 脱敏
	AppSecretConfigured bool   `json:"app_secret_configured"`
}
type notifyWXConfig struct {
	Enabled            bool   `json:"enabled"`
	AccountID          string `json:"account_id"`
	Target             string `json:"target"`
	BaseURL            string `json:"base_url"`
	BotToken           string `json:"bot_token"` // 脱敏
	BotTokenConfigured bool   `json:"bot_token_configured"`
}
type notifyWebhookConfig struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
}
type notifyDingTalkConfig struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
}
type notifyFeishuConfig struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
}
type notifyEmailConfig struct {
	Enabled            bool   `json:"enabled"`
	Host               string `json:"host"`
	Port               int    `json:"port"`
	Username           string `json:"username"`
	Password           string `json:"password"` // 脱敏
	PasswordConfigured bool   `json:"password_configured"`
	From               string `json:"from"`
	To                 string `json:"to"`
}

// 批次6 补全渠道配置（GET 回显；秘密脱敏 + *_configured）。
type notifyPushPlusConfig struct {
	Enabled         bool   `json:"enabled"`
	Token           string `json:"token"` // 脱敏
	TokenConfigured bool   `json:"token_configured"`
}
type notifyPushDeerConfig struct {
	Enabled           bool   `json:"enabled"`
	Server            string `json:"server"`
	Pushkey           string `json:"pushkey"` // 脱敏
	PushkeyConfigured bool   `json:"pushkey_configured"`
}
type notifyNtfyConfig struct {
	Enabled         bool   `json:"enabled"`
	Server          string `json:"server"`
	Topic           string `json:"topic"`
	Token           string `json:"token"` // 脱敏
	TokenConfigured bool   `json:"token_configured"`
}
type notifyGotifyConfig struct {
	Enabled         bool   `json:"enabled"`
	Server          string `json:"server"`
	Token           string `json:"token"` // 脱敏
	TokenConfigured bool   `json:"token_configured"`
}
type notifyAppriseConfig struct {
	Enabled          bool   `json:"enabled"`
	Server           string `json:"server"`
	Apikey           string `json:"apikey"` // 脱敏
	ApikeyConfigured bool   `json:"apikey_configured"`
}
type notifyWorkWechatConfig struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
}
type notifyWxPusherConfig struct {
	Enabled            bool   `json:"enabled"`
	UID                string `json:"uid"`
	TopicID            string `json:"topic_id"`
	AppToken           string `json:"app_token"` // 脱敏
	AppTokenConfigured bool   `json:"app_token_configured"`
}
type notifyLarkConfig struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
}
type notifyDiscordConfig struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
}
type notifyMQTTConfig struct {
	Enabled            bool   `json:"enabled"`
	Broker             string `json:"broker"`
	Topic              string `json:"topic"`
	Username           string `json:"username"`
	Password           string `json:"password"` // 脱敏
	PasswordConfigured bool   `json:"password_configured"`
	ClientID           string `json:"client_id"`
}
type notifyServerChanConfig struct {
	Enabled       bool   `json:"enabled"`
	Key           string `json:"key"` // 脱敏
	KeyConfigured bool   `json:"key_configured"`
}
type notifyConfigResp struct {
	Bark       notifyBarkConfig       `json:"bark"`
	Telegram   notifyTGConfig         `json:"telegram"`
	QQ         notifyQQConfig         `json:"qq"`
	WeChat     notifyWXConfig         `json:"wechat"`
	Webhook    notifyWebhookConfig    `json:"webhook"`
	DingTalk   notifyDingTalkConfig   `json:"dingtalk"`
	Feishu     notifyFeishuConfig     `json:"feishu"`
	Email      notifyEmailConfig      `json:"email"`
	PushPlus   notifyPushPlusConfig   `json:"pushplus"`
	PushDeer   notifyPushDeerConfig   `json:"pushdeer"`
	Ntfy       notifyNtfyConfig       `json:"ntfy"`
	Gotify     notifyGotifyConfig     `json:"gotify"`
	Apprise    notifyAppriseConfig    `json:"apprise"`
	WorkWechat notifyWorkWechatConfig `json:"workwechat"`
	WxPusher   notifyWxPusherConfig   `json:"wxpusher"`
	Lark       notifyLarkConfig       `json:"lark"`
	Discord    notifyDiscordConfig    `json:"discord"`
	MQTT       notifyMQTTConfig       `json:"mqtt"`
	ServerChan notifyServerChanConfig `json:"serverchan"`
}
