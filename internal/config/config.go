// Package config loads and saves ~/.scry/config.json.
//
// 자격증명(email/token)과 사이트 설정이 함께 살지만, 자격증명은 절대
// DB·로그·스냅샷에 들어가지 않는다 (constitution 제8조). 파일 모드 0600.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Member 는 설정으로 주입하는 정적 멤버 디렉터리 한 명분.
// bootstrap 의 members[] 에 머지되어 아바타 링/툴팁/파트 프리셋을 살린다.
type Member struct {
	Email         string `json:"email"`
	Name          string `json:"name,omitempty"`
	DisplayName   string `json:"display_name,omitempty"`
	Group         string `json:"group,omitempty"`
	Department    string `json:"department,omitempty"`
	JobRole       string `json:"job_role,omitempty"`
	JiraAccountID string `json:"jira_account_id,omitempty"`
	AvatarURL     string `json:"avatar_url,omitempty"`
}

// GroupRule 은 이슈→그룹 분류 룰. 위에서부터 첫 매치가 이긴다.
// 각 조건은 AND, 조건 내 목록은 OR. 비어 있는 조건은 항상 참.
type GroupRule struct {
	Group      string   `json:"group"`
	Projects   []string `json:"projects,omitempty"`
	Labels     []string `json:"labels,omitempty"`
	Components []string `json:"components,omitempty"`
}

// Product 는 그룹→제품 버킷 매핑 값.
type Product struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type Config struct {
	// 자격증명 + 연결 대상. Token 은 이 파일 밖으로 절대 복사하지 않는다.
	Site     string   `json:"site,omitempty"` // https://your-site.atlassian.net
	Email    string   `json:"email,omitempty"`
	Token    string   `json:"token,omitempty"`
	Projects []string `json:"projects,omitempty"`

	// 자격증명 검증 결과. `PUT credential/` 이 /myself 로 확인한 시각과 소유자 이름이며,
	// 토큰 원문과 달리 응답에 실려도 된다.
	TokenVerifiedAt string `json:"tokenVerifiedAt,omitempty"`
	TokenOwner      string `json:"tokenOwner,omitempty"`

	// sync 필드 매핑 (contracts/sync.md "Field mapping")
	FieldMap   map[string]string `json:"fieldMap,omitempty"`   // alias -> customfield_xxxxx
	BodyFields []string          `json:"bodyFields,omitempty"` // FTS 에 합칠 ADF 커스텀필드 id

	// 쓰기 allowlist: alias -> field id. 비어 있으면 인라인 편집 UI 자체가 숨는다.
	EditableFields map[string]string `json:"editableFields,omitempty"`

	// 회사 특화 표면 복원 (전부 선택)
	Members        []Member           `json:"members,omitempty"`
	GroupRules     []GroupRule        `json:"groupRules,omitempty"`
	GroupLabels    map[string]string  `json:"groupLabels,omitempty"`
	GroupColors    map[string]string  `json:"groupColors,omitempty"`
	ProductByGroup map[string]Product `json:"productByGroup,omitempty"`
	Features       map[string]bool    `json:"features,omitempty"` // presence/feed/push/deploy/qa/teamGroups
	QaDashboardURL string             `json:"qaDashboardUrl,omitempty"`

	StaleThresholdHours int `json:"staleThresholdHours,omitempty"` // 0 = 프론트 기본(72)

	// sync 주기 (초). 0 = 기본 (incremental 60, reconcile 3600)
	SyncIntervalSec      int `json:"syncIntervalSec,omitempty"`
	ReconcileIntervalSec int `json:"reconcileIntervalSec,omitempty"`
}

// profile 은 SetProfile(--profile 플래그) 또는 SCRY_PROFILE 로 정한다.
// 빈 값/"default" 는 기본 프로필(~/.scry 루트), 그 외는 ~/.scry/profiles/<이름>.
// 프로필마다 config.json 과 scry.db 가 통째로 분리되므로 회사용/데모용
// 사이트를 자격증명·미러 충돌 없이 오갈 수 있다.
var profile = os.Getenv("SCRY_PROFILE")

// SetProfile 은 CLI 의 --profile 플래그가 부른다. 환경변수보다 우선.
func SetProfile(name string) { profile = name }

// Profile 은 현재 활성 프로필 이름("" = 기본).
func Profile() string {
	if profile == "default" {
		return ""
	}
	return profile
}

// Dir 은 SCRY_HOME 또는 ~/.scry, 프로필이 있으면 그 아래 profiles/<이름>.
func Dir() (string, error) {
	base := os.Getenv("SCRY_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".scry")
	}
	if p := Profile(); p != "" {
		return filepath.Join(base, "profiles", p), nil
	}
	return base, nil
}

// DBPath 는 활성 프로필의 기본 SQLite 경로.
func DBPath() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "scry.db"), nil
}

// Profiles 는 존재하는 프로필 이름 목록 (기본 프로필 제외).
func Profiles() ([]string, error) {
	base := os.Getenv("SCRY_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		base = filepath.Join(home, ".scry")
	}
	entries, err := os.ReadDir(filepath.Join(base, "profiles"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

func Path() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.json"), nil
}

// Load 는 파일이 없으면 빈 Config 를 돌려준다 (에러 아님).
func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	return &c, nil
}

// Save 는 0600 으로 원자적으로 쓴다.
func (c *Config) Save() error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// HasCredential 은 쓰기/첨부 프록시가 가능한 상태인지.
func (c *Config) HasCredential() bool {
	return c.Site != "" && c.Email != "" && c.Token != ""
}
