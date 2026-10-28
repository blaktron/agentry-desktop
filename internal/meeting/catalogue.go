package meeting

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
)

// The catalogue is data: each language profile names its models per
// hardware tier, and each model its file, its source and its SHA-256. A new
// language profile is an entry here, not a change to the code.
//
//go:embed catalogue.json
var catalogueJSON []byte

// Asset is a file the feature downloads, checked against its SHA-256.
type Asset struct {
	Name   string `json:"name,omitempty"`
	File   string `json:"file"`
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Profile is a language profile: the language Whisper is told, and the
// models for each hardware tier. A tier's first model is its default; the
// next is what a computer too slow for the first falls back to.
type Profile struct {
	ID       string              `json:"id"`
	Name     string              `json:"name"`
	Language string              `json:"language"`
	Tiers    map[string][]string `json:"tiers"`
}

// The hardware tiers.
const (
	TierCPU   = "cpu"
	TierGPU   = "gpu"
	TierMetal = "metal"
)

// Catalogue is the parsed catalogue.
type Catalogue struct {
	VAD         Asset `json:"vad"`
	Calibration Asset `json:"calibration"`
	// Diarization are the two models that telling speakers in the room
	// apart runs with.
	Diarization DiarizationModels `json:"diarization"`
	Models      map[string]Asset  `json:"models"`
	Profiles    []Profile         `json:"profiles"`
}

// DefaultProfile is the language profile used until the person picks one.
const DefaultProfile = "english"

// LoadCatalogue parses the embedded catalogue and checks it: every model a
// profile names exists, and every asset has an https source and a SHA-256.
func LoadCatalogue() (Catalogue, error) {
	var c Catalogue
	if err := json.Unmarshal(catalogueJSON, &c); err != nil {
		return Catalogue{}, fmt.Errorf("meeting notes catalogue: %w", err)
	}
	return c, c.check()
}

// DiarizationModels are the speech segmentation and the voice embedding
// models the speaker separation runs with.
type DiarizationModels struct {
	Segmentation Asset `json:"segmentation"`
	Embedding    Asset `json:"embedding"`
}

func (c Catalogue) check() error {
	assets := []Asset{c.VAD, c.Calibration, c.Diarization.Segmentation, c.Diarization.Embedding}
	for _, a := range c.Models {
		assets = append(assets, a)
	}
	for _, a := range assets {
		u, err := url.Parse(a.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return fmt.Errorf("meeting notes catalogue: %s has no https source", a.File)
		}
		if len(a.SHA256) != 64 || a.Size <= 0 || a.File == "" {
			return fmt.Errorf("meeting notes catalogue: %s has no SHA-256 or size", a.File)
		}
	}
	if len(c.Profiles) == 0 {
		return fmt.Errorf("meeting notes catalogue: no language profile")
	}
	for _, p := range c.Profiles {
		if len(p.Tiers[TierCPU]) == 0 {
			return fmt.Errorf("meeting notes catalogue: profile %s has no CPU model", p.ID)
		}
		for _, ids := range p.Tiers {
			for _, id := range ids {
				if _, ok := c.Models[id]; !ok {
					return fmt.Errorf("meeting notes catalogue: profile %s names %s, which is not a model", p.ID, id)
				}
			}
		}
	}
	return nil
}

// Profile looks a profile up by id; an unknown id is the default profile.
func (c Catalogue) Profile(id string) Profile {
	for _, p := range c.Profiles {
		if p.ID == id {
			return p
		}
	}
	for _, p := range c.Profiles {
		if p.ID == DefaultProfile {
			return p
		}
	}
	return c.Profiles[0]
}

// Choices are the models a profile offers on a tier, best first.
func (c Catalogue) Choices(profile, tier string) []string {
	p := c.Profile(profile)
	if ids := p.Tiers[tier]; len(ids) > 0 {
		return ids
	}
	return p.Tiers[TierCPU]
}

// Allowed says whether model is one the profile offers on any tier: a
// model the person chose in Settings must be one of these.
func (c Catalogue) Allowed(profile, model string) bool {
	for _, ids := range c.Profile(profile).Tiers {
		if slices.Contains(ids, model) {
			return true
		}
	}
	return false
}
