package services

import (
	"context"
	"fmt"

	"ssk/internal/services/jira"
)

func (s *BriefingService) withJiraContext(ctx context.Context) *BriefingService {
	copyService := *s
	if client, ok := s.jiraClient.(*jira.Client); ok {
		copyService.jiraClient = client.WithContext(ctx)
	}
	return &copyService
}

func (s *BriefingService) inspectionTicket(ctx context.Context, key string, opts TicketInspectionOptions) (*jira.Ticket, error) {
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	var ticket *jira.Ticket
	var err error
	if client, ok := s.jiraClient.(interface {
		GetInspectionTicketByKey(string, bool) (*jira.Ticket, error)
	}); ok {
		include := opts.IncludeDevelopment == nil || *opts.IncludeDevelopment
		ticket, err = client.GetInspectionTicketByKey(key, include)
	} else {
		if opts.IncludeDevelopment != nil && !*opts.IncludeDevelopment {
			return nil, fmt.Errorf("jira client does not support skipping development enrichment")
		}
		ticket, err = s.jiraClient.GetBriefingTicketByKey(key)
	}
	if err != nil {
		return nil, err
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	return ticket, nil
}
