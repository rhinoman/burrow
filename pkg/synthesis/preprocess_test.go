package synthesis

import (
	"strings"
	"testing"
)

func TestPreprocessDataNonJSON(t *testing.T) {
	// Plain text should be returned unchanged.
	input := "This is plain text with some data: 42 degrees."
	got := PreprocessData(input)
	if got != input {
		t.Errorf("expected unchanged, got %q", got)
	}
}

func TestPreprocessDataEmptyString(t *testing.T) {
	got := PreprocessData("")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestPreprocessDataXML(t *testing.T) {
	input := `<?xml version="1.0"?><root><item>test</item></root>`
	got := PreprocessData(input)
	if got != input {
		t.Errorf("expected XML unchanged, got %q", got)
	}
}

func TestPreprocessDataSimpleObject(t *testing.T) {
	input := `{"temperature": 42, "condition": "Clear", "humidity": 67}`
	got := PreprocessData(input)

	if !strings.Contains(got, "temperature: 42") {
		t.Error("expected temperature: 42")
	}
	if !strings.Contains(got, "condition: Clear") {
		t.Error("expected condition: Clear")
	}
	if !strings.Contains(got, "humidity: 67") {
		t.Error("expected humidity: 67")
	}
}

func TestPreprocessDataSkipsMetadata(t *testing.T) {
	input := `{
		"@context": "https://schema.org",
		"@type": "WeatherForecast",
		"icon": "https://api.weather.gov/icons/land/night/few",
		"temperature": 12,
		"correlationId": "abc123"
	}`
	got := PreprocessData(input)

	if strings.Contains(got, "@context") {
		t.Error("@context should be stripped")
	}
	if strings.Contains(got, "@type") {
		t.Error("@type should be stripped")
	}
	if strings.Contains(got, "icon") {
		t.Error("icon should be stripped")
	}
	if strings.Contains(got, "correlationId") {
		t.Error("correlationId should be stripped")
	}
	if !strings.Contains(got, "temperature: 12") {
		t.Error("expected temperature preserved")
	}
}

func TestPreprocessDataUnwrapsUnitCode(t *testing.T) {
	input := `{
		"temperature": {"value": 67, "unitCode": "wmoUnit:percent"},
		"windSpeed": {"value": 10, "unitCode": "wmoUnit:km_h-1"}
	}`
	got := PreprocessData(input)

	if !strings.Contains(got, "temperature: 67") {
		t.Errorf("expected unwrapped temperature, got:\n%s", got)
	}
	if !strings.Contains(got, "windSpeed: 10") {
		t.Errorf("expected unwrapped windSpeed, got:\n%s", got)
	}
	if strings.Contains(got, "unitCode") {
		t.Error("unitCode should be stripped")
	}
}

func TestPreprocessDataNestedObject(t *testing.T) {
	input := `{
		"location": {
			"city": "Fairbanks",
			"state": "AK"
		},
		"temp": 12
	}`
	got := PreprocessData(input)

	if !strings.Contains(got, "location:") {
		t.Error("expected location section header")
	}
	if !strings.Contains(got, "city: Fairbanks") {
		t.Error("expected city")
	}
	if !strings.Contains(got, "state: AK") {
		t.Error("expected state")
	}
	if !strings.Contains(got, "temp: 12") {
		t.Error("expected temp")
	}
}

func TestPreprocessDataArray(t *testing.T) {
	input := `{
		"items": [
			{"title": "Article One", "link": "https://example.com/1"},
			{"title": "Article Two", "link": "https://example.com/2"}
		]
	}`
	got := PreprocessData(input)

	if !strings.Contains(got, "Article One") {
		t.Error("expected Article One")
	}
	if !strings.Contains(got, "Article Two") {
		t.Error("expected Article Two")
	}
	if !strings.Contains(got, "https://example.com/1") {
		t.Error("expected link preserved")
	}
}

// NWS-style weather forecast period — this is the primary use case.
func TestPreprocessDataNWSForecast(t *testing.T) {
	input := `{
		"@context": ["https://geojson.org/geojson-ld/geojson-context.jsonld"],
		"type": "Feature",
		"geometry": {"type": "Polygon", "coordinates": [[[1,2],[3,4]]]},
		"properties": {
			"updated": "2024-02-24T10:00:00Z",
			"elevation": {"value": 133, "unitCode": "wmoUnit:m"},
			"periods": [
				{
					"number": 1,
					"name": "Tonight",
					"temperature": 12,
					"temperatureUnit": "F",
					"windSpeed": "10 mph",
					"windDirection": "N",
					"shortForecast": "Mostly Clear",
					"detailedForecast": "Mostly clear. Low around 12. North wind around 10 mph.",
					"relativeHumidity": {"value": 67, "unitCode": "wmoUnit:percent"},
					"icon": "https://api.weather.gov/icons/land/night/few"
				}
			]
		}
	}`
	got := PreprocessData(input)

	// Metadata should be gone
	if strings.Contains(got, "@context") {
		t.Error("@context should be stripped")
	}
	if strings.Contains(got, "geometry") {
		t.Error("geometry should be stripped")
	}

	// Key data should be present
	if !strings.Contains(got, "Tonight") {
		t.Error("expected period name 'Tonight'")
	}
	if !strings.Contains(got, "temperature: 12") {
		t.Error("expected temperature")
	}
	if !strings.Contains(got, "10 mph") {
		t.Error("expected wind speed")
	}
	if !strings.Contains(got, "Mostly Clear") || !strings.Contains(got, "Mostly clear") {
		t.Error("expected forecast text")
	}

	// Unit-code wrapper should be unwrapped
	if !strings.Contains(got, "relativeHumidity: 67") {
		t.Errorf("expected unwrapped humidity, got:\n%s", got)
	}

	// Should be significantly shorter than input
	if len(got) >= len(input) {
		t.Errorf("expected shorter output: input=%d bytes, output=%d bytes", len(input), len(got))
	}
}

func TestPreprocessDataRSSItems(t *testing.T) {
	input := `{
		"items": [
			{
				"title": "Breaking: Market Drops 5%",
				"link": "https://news.example.com/market-drops",
				"description": "Markets plunged 5% today in heavy trading.",
				"pubDate": "2024-02-24T15:30:00Z"
			},
			{
				"title": "Tech Earnings Surprise",
				"link": "https://news.example.com/tech-earnings",
				"description": "Major tech company beats estimates by 20%.",
				"pubDate": "2024-02-24T14:00:00Z"
			}
		]
	}`
	got := PreprocessData(input)

	if !strings.Contains(got, "Breaking: Market Drops 5%") {
		t.Error("expected first article title")
	}
	if !strings.Contains(got, "https://news.example.com/market-drops") {
		t.Error("expected first article link")
	}
	if !strings.Contains(got, "Tech Earnings Surprise") {
		t.Error("expected second article title")
	}
}

func TestPreprocessDataScalarArray(t *testing.T) {
	input := `{"tags": ["urgent", "weather", "local"]}`
	got := PreprocessData(input)

	if !strings.Contains(got, "urgent") {
		t.Error("expected 'urgent'")
	}
	if !strings.Contains(got, "weather") {
		t.Error("expected 'weather'")
	}
}

func TestPreprocessDataBooleanAndNull(t *testing.T) {
	input := `{"active": true, "expired": false, "notes": null}`
	got := PreprocessData(input)

	if !strings.Contains(got, "active: true") {
		t.Error("expected active: true")
	}
	if !strings.Contains(got, "expired: false") {
		t.Error("expected expired: false")
	}
}

func TestPreprocessDataFloatValues(t *testing.T) {
	input := `{"price": 29.99, "quantity": 100}`
	got := PreprocessData(input)

	if !strings.Contains(got, "price: 29.99") {
		t.Error("expected price: 29.99")
	}
	if !strings.Contains(got, "quantity: 100") {
		t.Error("expected quantity: 100")
	}
}

func TestPreprocessDataTopLevelArray(t *testing.T) {
	input := `[{"name": "Alice"}, {"name": "Bob"}]`
	got := PreprocessData(input)

	if !strings.Contains(got, "Alice") {
		t.Error("expected Alice")
	}
	if !strings.Contains(got, "Bob") {
		t.Error("expected Bob")
	}
}

func TestPreprocessDataTopLevelScalar(t *testing.T) {
	input := `"just a string"`
	got := PreprocessData(input)
	if !strings.Contains(got, "just a string") {
		t.Errorf("expected scalar string, got %q", got)
	}
}

func TestPreprocessDataPreservesURLs(t *testing.T) {
	input := `{
		"title": "Article",
		"url": "https://example.com/path?q=test&page=1",
		"link": "https://other.com/article#section"
	}`
	got := PreprocessData(input)

	if !strings.Contains(got, "https://example.com/path?q=test&page=1") {
		t.Error("expected URL preserved exactly")
	}
	if !strings.Contains(got, "https://other.com/article#section") {
		t.Error("expected link preserved exactly")
	}
}

func TestPreprocessDataUSGSEarthquake(t *testing.T) {
	input := `{
		"type": "Feature",
		"properties": {
			"mag": 4.2,
			"place": "65 km NNW of Anchorage, Alaska",
			"time": 1708819200000,
			"url": "https://earthquake.usgs.gov/earthquakes/eventpage/ak024",
			"title": "M 4.2 - 65km NNW of Anchorage, Alaska",
			"alert": null
		},
		"geometry": {"type": "Point", "coordinates": [-149.5, 61.8, 48.2]}
	}`
	got := PreprocessData(input)

	if !strings.Contains(got, "mag: 4.2") {
		t.Error("expected magnitude")
	}
	if !strings.Contains(got, "65 km NNW of Anchorage") {
		t.Error("expected place")
	}
	if !strings.Contains(got, "https://earthquake.usgs.gov") {
		t.Error("expected URL preserved")
	}
	if strings.Contains(got, "geometry") {
		t.Error("geometry should be stripped")
	}
}
