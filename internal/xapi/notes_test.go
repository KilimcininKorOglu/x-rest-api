package xapi

import (
	"encoding/json"
	"testing"
)

// notesFixture mirrors the BirdwatchFetchNotes shape with synthetic content: one
// misleading note carrying tags and a cited source, and one not-misleading note.
const notesFixture = `{"data":{"tweet_result_by_rest_id":{"result":{
  "can_user_write_notes_on_post_author": true,
  "misleading_birdwatch_notes": {"notes": [{
    "rest_id": "5001",
    "rating_status": "NeedsMoreRatings",
    "decided_by": "ExampleModel (v1.0)",
    "is_media_note": false,
    "language": "en",
    "created_at": 1700000000000,
    "birdwatch_profile": {"alias": "example-alias-one"},
    "data_v1": {
      "classification": "MisinformedOrPotentiallyMisleading",
      "misleading_tags": ["MissingImportantContext", "OutdatedInformation"],
      "trustworthy_sources": true,
      "summary": {
        "text": "This claim needs context.\n\nsource: example.invalid",
        "entities": [{"fromIndex": 10, "toIndex": 20,
          "ref": {"type": "TimelineUrl", "url": "https://t.co/example1", "urlType": "ExternalUrl"}}]
      }
    }
  }], "slice_info": {}},
  "not_misleading_birdwatch_notes": {"notes": [{
    "rest_id": "5002",
    "rating_status": "CurrentlyRatedHelpful",
    "birdwatch_profile": {"alias": "example-alias-two"},
    "data_v1": {
      "classification": "NotMisleading",
      "not_misleading_tags": ["OutdatedNowButNotWhenWritten"],
      "summary": {"text": "The post is accurate."}
    }
  }], "slice_info": {}}
}}}}`

// notesResult decodes the fixture down to the result node the parser reads.
func notesResult(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(notesFixture), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	res := asMap(dig(m, "data", "tweet_result_by_rest_id", "result"))
	if res == nil {
		t.Fatal("fixture has no result node")
	}
	return res
}

// TestParseNotesMisleading checks the misleading group, whose tags live under
// misleading_tags and whose sources come from the summary entities.
func TestParseNotesMisleading(t *testing.T) {
	got := parseNoteList(dig(notesResult(t), "misleading_birdwatch_notes", "notes"))
	if len(got) != 1 {
		t.Fatalf("parsed %d misleading notes, want 1", len(got))
	}
	n := got[0]
	if len(n.Tags) != 2 {
		t.Fatalf("Tags = %v, want two entries", n.Tags)
	}
	if len(n.Sources) != 1 {
		t.Fatalf("Sources = %v, want one entry", n.Sources)
	}
	checkFields(t, []fieldCheck{
		{"RestID", n.RestID, "5001"},
		{"Text", n.Text, "This claim needs context.\n\nsource: example.invalid"},
		{"Classification", n.Classification, "MisinformedOrPotentiallyMisleading"},
		{"Tags[0]", n.Tags[0], "MissingImportantContext"},
		{"RatingStatus", n.RatingStatus, "NeedsMoreRatings"},
		{"DecidedBy", n.DecidedBy, "ExampleModel (v1.0)"},
		{"TrustworthySources", n.TrustworthySources, true},
		{"Language", n.Language, "en"},
		{"CreatedAt", n.CreatedAt, int64(1700000000000)},
		{"AuthorAlias", n.AuthorAlias, "example-alias-one"},
		{"Sources[0].URL", n.Sources[0].URL, "https://t.co/example1"},
	})
}

// TestParseNotesNotMisleading checks the other group, whose tags live under a
// differently named field.
func TestParseNotesNotMisleading(t *testing.T) {
	got := parseNoteList(dig(notesResult(t), "not_misleading_birdwatch_notes", "notes"))
	if len(got) != 1 {
		t.Fatalf("parsed %d not-misleading notes, want 1", len(got))
	}
	n := got[0]
	if len(n.Tags) != 1 {
		t.Fatalf("Tags = %v, want one entry", n.Tags)
	}
	checkFields(t, []fieldCheck{
		{"RestID", n.RestID, "5002"},
		{"Classification", n.Classification, "NotMisleading"},
		{"Tags[0]", n.Tags[0], "OutdatedNowButNotWhenWritten"},
		{"RatingStatus", n.RatingStatus, "CurrentlyRatedHelpful"},
		{"AuthorAlias", n.AuthorAlias, "example-alias-two"},
		{"Sources", len(n.Sources), 0},
	})
}

// TestParseNoteRejectsIdless verifies a note with no rest_id is dropped rather
// than returned half-built.
func TestParseNoteRejectsIdless(t *testing.T) {
	if got := parseNote(nil); got != nil {
		t.Errorf("parseNote(nil) = %+v, want nil", got)
	}
	if got := parseNote(map[string]any{"rating_status": "NeedsMoreRatings"}); got != nil {
		t.Errorf("parseNote without rest_id = %+v, want nil", got)
	}
}
