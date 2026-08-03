package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"
)

const (
	pluginID      = "warp-egress"
	pluginVersion = "0.2.7"
	schemaVersion = 1
	abiVersion    = 1
)

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type lifecycleRequest struct {
	ConfigYAML string `json:"config_yaml"`
}

type configField struct {
	Name        string   `json:"Name"`
	Type        string   `json:"Type"`
	EnumValues  []string `json:"EnumValues,omitempty"`
	Description string   `json:"Description"`
}

type pluginMetadata struct {
	Name             string        `json:"Name"`
	Version          string        `json:"Version"`
	Author           string        `json:"Author"`
	GitHubRepository string        `json:"GitHubRepository"`
	Logo             string        `json:"Logo"`
	ConfigFields     []configField `json:"ConfigFields"`
}

type registrationCapabilities struct {
	ManagementAPI bool `json:"management_api"`
}

type registration struct {
	SchemaVersion uint32                   `json:"schema_version"`
	Metadata      pluginMetadata           `json:"metadata"`
	Capabilities  registrationCapabilities `json:"capabilities"`
}

type managementRoute struct {
	Method      string `json:"Method"`
	Path        string `json:"Path"`
	Menu        string `json:"Menu,omitempty"`
	Description string `json:"Description,omitempty"`
}

type resourceRoute struct {
	Path        string `json:"Path"`
	Menu        string `json:"Menu,omitempty"`
	Description string `json:"Description,omitempty"`
}

type managementRegistration struct {
	Routes    []managementRoute `json:"routes,omitempty"`
	Resources []resourceRoute   `json:"resources,omitempty"`
}

type managementRequest struct {
	Method         string      `json:"Method"`
	Path           string      `json:"Path"`
	Headers        http.Header `json:"Headers"`
	Query          url.Values  `json:"Query"`
	Body           []byte      `json:"Body"`
	HostCallbackID string      `json:"host_callback_id,omitempty"`
}

type managementResponse struct {
	StatusCode int         `json:"StatusCode"`
	Headers    http.Header `json:"Headers"`
	Body       []byte      `json:"Body"`
}

type hostAuthFileEntry struct {
	ID          string            `json:"id"`
	AuthIndex   string            `json:"auth_index"`
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	Provider    string            `json:"provider"`
	Label       string            `json:"label"`
	Email       string            `json:"email"`
	Disabled    bool              `json:"disabled"`
	RuntimeOnly bool              `json:"runtime_only"`
	ProxyURL    string            `json:"proxy_url"`
	Attributes  map[string]string `json:"attributes,omitempty"`
}

type hostAuthListResponse struct {
	Files []hostAuthFileEntry `json:"files"`
}

type hostAuthGetRequest struct {
	AuthIndex string `json:"auth_index"`
}

type hostAuthGetResponse struct {
	Name      string          `json:"name"`
	AuthIndex string          `json:"auth_index"`
	JSON      json.RawMessage `json:"json"`
}

type hostAuthSaveRequest struct {
	Name string          `json:"name"`
	JSON json.RawMessage `json:"json"`
}

type hostAuthSaveResponse struct {
	Status string `json:"status"`
	Name   string `json:"name,omitempty"`
}

type ProfileMode string

const (
	ProfileModeManaged  ProfileMode = "managed"
	ProfileModeExternal ProfileMode = "external"
)

type Profile struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Mode        ProfileMode `json:"mode"`
	ProxyURL    string      `json:"proxy_url"`
	ListenHost  string      `json:"listen_host,omitempty"`
	ListenPort  int         `json:"listen_port,omitempty"`
	Directory   string      `json:"directory,omitempty"`
	PID         int         `json:"pid,omitempty"`
	Running     bool        `json:"running"`
	Healthy     bool        `json:"healthy"`
	ExitIP      string      `json:"exit_ip,omitempty"`
	Colo        string      `json:"colo,omitempty"`
	WarpMode    string      `json:"warp_mode,omitempty"`
	LatencyMS   int64       `json:"latency_ms,omitempty"`
	LastChecked time.Time   `json:"last_checked,omitempty"`
	LastError   string      `json:"last_error,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

type TypeRule struct {
	Key       string `json:"key"`
	ProfileID string `json:"profile_id"`
	Enabled   bool   `json:"enabled"`
}

type RegexRule struct {
	ID        string `json:"id"`
	Pattern   string `json:"pattern"`
	Target    string `json:"target"`
	ProfileID string `json:"profile_id"`
	Enabled   bool   `json:"enabled"`
}

type Rules struct {
	GlobalProfileID string            `json:"global_profile_id"`
	TypeRules       []TypeRule        `json:"type_rules"`
	RegexRules      []RegexRule       `json:"regex_rules"`
	ExactRules      map[string]string `json:"exact_rules"`
}

type AutoSwitchConfig struct {
	Enabled               bool      `json:"enabled"`
	FailoverEnabled       bool      `json:"failover_enabled"`
	RotateIntervalSeconds int       `json:"rotate_interval_seconds"`
	RequireDifferentIP    bool      `json:"require_different_ip"`
	LastSwitchAt          time.Time `json:"last_switch_at,omitempty"`
	LastProfileID         string    `json:"last_profile_id,omitempty"`
	LastReason            string    `json:"last_reason,omitempty"`
}

type PersistedState struct {
	Version  int              `json:"version"`
	Profiles []*Profile       `json:"profiles"`
	Rules    Rules            `json:"rules"`
	Auto     AutoSwitchConfig `json:"auto_switch"`
}

type EffectiveRoute struct {
	ProfileID string `json:"profile_id,omitempty"`
	RuleType  string `json:"rule_type"`
	RuleKey   string `json:"rule_key,omitempty"`
	ProxyURL  string `json:"proxy_url,omitempty"`
}

type AuthFileView struct {
	hostAuthFileEntry
	Effective EffectiveRoute `json:"effective"`
}

type ApplyItemResult struct {
	AuthIndex string `json:"auth_index"`
	Name      string `json:"name"`
	RuleType  string `json:"rule_type,omitempty"`
	ProfileID string `json:"profile_id,omitempty"`
	ProxyURL  string `json:"proxy_url,omitempty"`
	Changed   bool   `json:"changed"`
	Skipped   bool   `json:"skipped"`
	Error     string `json:"error,omitempty"`
}

type ApplyRulesResult struct {
	Total   int               `json:"total"`
	Changed int               `json:"changed"`
	Skipped int               `json:"skipped"`
	Failed  int               `json:"failed"`
	Items   []ApplyItemResult `json:"items"`
}

type createProfileRequest struct {
	Name      string `json:"name"`
	Mode      string `json:"mode"`
	ProxyURL  string `json:"proxy_url,omitempty"`
	AutoStart bool   `json:"auto_start"`
}

type importProfileRequest struct {
	Name         string `json:"name"`
	WGCFProfile  string `json:"wgcf_profile"`
}

type profileActionRequest struct {
	ID     string `json:"id"`
	Action string `json:"action"`
}

type profileDeleteRequest struct {
	ID string `json:"id"`
}

type globalSwitchRequest struct {
	ProfileID string `json:"profile_id"`
}

type exactAssignRequest struct {
	AuthIndex string `json:"auth_index"`
	ProfileID string `json:"profile_id"`
	ApplyNow  bool   `json:"apply_now"`
}

type statusResponse struct {
	PluginID             string              `json:"plugin_id"`
	Version              string              `json:"version"`
	GlobalRelayURL       string              `json:"global_relay_url"`
	GlobalRelayRunning   bool                `json:"global_relay_running"`
	GlobalProfileID      string              `json:"global_profile_id,omitempty"`
	GlobalProfile        *Profile            `json:"global_profile,omitempty"`
	Profiles             []*Profile          `json:"profiles"`
	DuplicateExitIPs     map[string][]string `json:"duplicate_exit_ips,omitempty"`
	DataDir              string              `json:"data_dir"`
	LastError            string              `json:"last_error,omitempty"`
	RequiredHostProxyURL string              `json:"required_host_proxy_url"`
	AutoSwitch           AutoSwitchConfig    `json:"auto_switch"`
}
