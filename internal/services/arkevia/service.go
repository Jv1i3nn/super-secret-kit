package arkevia

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"ssk/internal/models"
)

// maxPeriodSearchRows is the number of recent documents fetched when looking up
// documents by year or month. It is large enough to cover several years of
// monthly documents while keeping a single request.
const maxPeriodSearchRows = 500

// documentDateRe extracts the YYYY-MM-DD date embedded in Arkevia filenames
// (e.g. "John Doe_Bulletin de paie du 2026-05-31_2.pdf").
var documentDateRe = regexp.MustCompile(`(\d{4})-(\d{2})-\d{2}`)

type Service struct {
	client *Client
}

func NewService(credentials *models.Credentials, tokenDir string) *Service {
	client := NewClient(credentials, tokenDir)
	return &Service{
		client: client,
	}
}

func (s *Service) GetDocumentList(totalRequested int) ([]Document, error) {
	return s.client.GetDocumentList(totalRequested)
}

func (s *Service) DownloadDocument(document Document, outputDir string) (string, error) {
	content, err := s.client.DownloadDocument(document)
	if err != nil {
		return "", fmt.Errorf("failed to download document: %w", err)
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	outputPath := filepath.Join(outputDir, document.Filename)
	if err := os.WriteFile(outputPath, content, 0644); err != nil {
		return "", fmt.Errorf("failed to save document: %w", err)
	}

	return outputPath, nil
}

// FindDocuments returns every document whose embedded date matches the given
// year, and month too when month is non-zero. A single month can hold several
// documents (e.g. payslip split in two), so all matches are returned.
func (s *Service) FindDocuments(year, month int) ([]Document, error) {
	documents, err := s.GetDocumentList(maxPeriodSearchRows)
	if err != nil {
		return nil, fmt.Errorf("failed to get document list: %w", err)
	}

	var matched []Document
	for i := range documents {
		if documentMatchesPeriod(documents[i].Filename, year, month) {
			matched = append(matched, documents[i])
		}
	}

	return matched, nil
}

// documentMatchesPeriod reports whether the date embedded in filename matches
// the given year, and month too when month is non-zero (1-12).
func documentMatchesPeriod(filename string, year, month int) bool {
	m := documentDateRe.FindStringSubmatch(filename)
	if m == nil {
		return false
	}

	docYear, _ := strconv.Atoi(m[1])
	if docYear != year {
		return false
	}

	if month != 0 {
		docMonth, _ := strconv.Atoi(m[2])
		if docMonth != month {
			return false
		}
	}

	return true
}

// DownloadByPeriod downloads every document matching the given year, and month
// too when month is non-zero. It returns the saved file paths.
func (s *Service) DownloadByPeriod(year, month int, outputDir string) ([]string, error) {
	documents, err := s.FindDocuments(year, month)
	if err != nil {
		return nil, err
	}

	if len(documents) == 0 {
		if month != 0 {
			return nil, fmt.Errorf("no document found for %04d-%02d", year, month)
		}
		return nil, fmt.Errorf("no document found for %04d", year)
	}

	paths := make([]string, 0, len(documents))
	for i := range documents {
		path, err := s.DownloadDocument(documents[i], outputDir)
		if err != nil {
			return paths, fmt.Errorf("failed to download %q: %w", documents[i].Filename, err)
		}
		paths = append(paths, path)
	}

	return paths, nil
}

func (s *Service) DownloadLatest(outputDir string) (string, error) {
	documents, err := s.GetDocumentList(1)
	if err != nil {
		return "", fmt.Errorf("failed to get document list: %w", err)
	}

	if len(documents) == 0 {
		return "", fmt.Errorf("no documents found")
	}

	return s.DownloadDocument(documents[0], outputDir)
}
