package main

import (
	"testing"
)

func TestParseServiceQueryFull(t *testing.T) {
	svc, tool, params := parseServiceQuery("search sam-gov search_opportunities naics=541370 status=active")
	if svc != "sam-gov" {
		t.Errorf("expected svc=sam-gov, got %q", svc)
	}
	if tool != "search_opportunities" {
		t.Errorf("expected tool=search_opportunities, got %q", tool)
	}
	if params["naics"] != "541370" {
		t.Errorf("expected naics=541370, got %q", params["naics"])
	}
	if params["status"] != "active" {
		t.Errorf("expected status=active, got %q", params["status"])
	}
}

func TestParseServiceQueryNoParams(t *testing.T) {
	svc, tool, params := parseServiceQuery("search edgar company_filings")
	if svc != "edgar" {
		t.Errorf("expected svc=edgar, got %q", svc)
	}
	if tool != "company_filings" {
		t.Errorf("expected tool=company_filings, got %q", tool)
	}
	if len(params) != 0 {
		t.Errorf("expected no params, got %v", params)
	}
}

func TestParseServiceQueryCommand(t *testing.T) {
	svc, tool, _ := parseServiceQuery("query edgar filings")
	if svc != "edgar" {
		t.Errorf("expected svc=edgar, got %q", svc)
	}
	if tool != "filings" {
		t.Errorf("expected tool=filings, got %q", tool)
	}
}

func TestParseServiceQueryServiceOnly(t *testing.T) {
	svc, tool, _ := parseServiceQuery("search myservice")
	if svc != "myservice" {
		t.Errorf("expected svc=myservice, got %q", svc)
	}
	if tool != "" {
		t.Errorf("expected empty tool, got %q", tool)
	}
}

func TestParseServiceQueryEmpty(t *testing.T) {
	svc, tool, _ := parseServiceQuery("search ")
	if svc != "" {
		t.Errorf("expected empty svc, got %q", svc)
	}
	if tool != "" {
		t.Errorf("expected empty tool, got %q", tool)
	}
}
