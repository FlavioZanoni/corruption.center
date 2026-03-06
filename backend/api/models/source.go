package models

import "time"

type SourceType string
type SourceReliability string

const (
	SourceTypeNewsOutlet       SourceType = "news_outlet"
	SourceTypeGovernmentAgency SourceType = "government_agency"
	SourceTypeCourtDocument    SourceType = "court_document"
	SourceTypeParliamentary    SourceType = "parliamentary"
	SourceTypeOfficialGazette  SourceType = "official_gazette"
	SourceTypeNGOWatchdog      SourceType = "ngo_watchdog"
	SourceTypeAcademic         SourceType = "academic"
)

const (
	ReliabilityHigh   SourceReliability = "high"
	ReliabilityMedium SourceReliability = "medium"
	ReliabilityLow    SourceReliability = "low"
)

type Source struct {
	ID            string            `json:"id"`
	URL           string            `json:"url"`
	Title         string            `json:"title"`
	Publisher     string            `json:"publisher"`
	Type          SourceType        `json:"type"`
	Reliability   SourceReliability `json:"reliability"`
	DatePublished *time.Time        `json:"date_published"`
	DateScraped   time.Time         `json:"date_scraped"`
	Active        bool              `json:"active"`
}
