// Writes. This is the one file in the package that is not a GET, and the
// surface is deliberately one call wide: one single-value, bundle-backed field
// on one issue. Read the write invariant in CLAUDE.md before adding a second.
package youtrack

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// fieldsEditable asks for a field, its current value and the bundle that value
// came from. Only the project field types backed by a bundle carry one, so a
// missing bundle is what marks a field as not editable here.
const fieldsEditable = "name,$type," + fieldsValue +
	",projectCustomField(bundle(values(name,archived),aggregatedUsers(login,fullName,banned)))"

// editableTypes maps a writable field type to the JSON key its values are
// addressed by. A type absent from this map is not offered and cannot be sent.
//
// Everything here is single-value and backed by a closed set the instance
// itself hands us. What is left out is left out on purpose: a multi-value
// field takes an array rather than one object, and text, date and period have
// no list of legal answers to offer at all.
var editableTypes = map[string]string{
	"StateIssueCustomField":         "name",
	"SingleEnumIssueCustomField":    "name",
	"SingleVersionIssueCustomField": "name",
	"SingleOwnedIssueCustomField":   "name",
	"SingleBuildIssueCustomField":   "name",
	// A user bundle is the same kind of closed set, only keyed differently:
	// two people can share a full name, so the write goes by login.
	"SingleUserIssueCustomField": "login",
}

// Editable is one field of an issue that SetField can write, along with the
// values its bundle allows.
type Editable struct {
	Name string
	Type string
	// Value is what the field reads now, rendered for display only, and in the
	// same shape as an Option's Label so the picker can find its way back to it.
	Value   string
	Options []Option
	// key is the JSON key the value is addressed by, from editableTypes.
	key string
}

// Option is one answer a field will accept. Label and Value differ for user
// fields, where the list reads as names and the write goes by login.
type Option struct {
	Label string
	Value string
}

// EditableFields returns the fields of an issue that can be set from a list.
// The answer is per issue rather than per project because the bundle is: two
// projects name the states of their workflow differently.
func (c *Client) EditableFields(ctx context.Context, id string) ([]Editable, error) {
	var raw []struct {
		// Embedded so name, $type and the value shapes are decoded and
		// rendered by the same code the read path uses.
		CustomField
		ProjectCustomField struct {
			Bundle struct {
				Values []struct {
					Name     string `json:"name"`
					Archived bool   `json:"archived"`
				} `json:"values"`
				// A user bundle has no `values`; its members are the users
				// aggregated from the groups and individuals it names.
				AggregatedUsers []struct {
					Login    string `json:"login"`
					FullName string `json:"fullName"`
					Banned   bool   `json:"banned"`
				} `json:"aggregatedUsers"`
			} `json:"bundle"`
		} `json:"projectCustomField"`
	}
	err := c.get(ctx, "/issues/"+url.PathEscape(id)+"/customFields",
		url.Values{"fields": {fieldsEditable}}, &raw)
	if err != nil {
		return nil, err
	}

	out := make([]Editable, 0, len(raw))
	for _, f := range raw {
		key, ok := editableTypes[f.Type]
		if !ok {
			continue
		}
		e := Editable{Name: f.Name, Type: f.Type, Value: f.String(), key: key}
		bundle := f.ProjectCustomField.Bundle
		for _, v := range bundle.Values {
			// An archived value keeps rendering on the issues that already
			// carry it, but the instance refuses it as a new one.
			if v.Name != "" && !v.Archived {
				e.Options = append(e.Options, Option{Label: v.Name, Value: v.Name})
			}
		}
		for _, u := range bundle.AggregatedUsers {
			// Banned is to a user what archived is to a bundle value.
			if u.Login == "" || u.Banned {
				continue
			}
			// Labelled the way the read path renders a user, so the value the
			// field already holds matches one of these rows.
			user := User{Login: u.Login, FullName: u.FullName}
			e.Options = append(e.Options, Option{Label: user.String(), Value: u.Login})
		}
		// A field with nothing to choose from is not an offer.
		if len(e.Options) > 0 {
			out = append(out, e)
		}
	}
	return out, nil
}

// SetField sets one custom field of one issue. Workflow rules run on the far
// side, so a value listed here can still come back rejected — that is a
// dialog, not a bug.
func (c *Client) SetField(ctx context.Context, id string, f Editable, value string) error {
	return c.post(ctx, "/issues/"+url.PathEscape(id), map[string]any{
		"customFields": []map[string]any{{
			"name": f.Name,
			// $type is what tells YouTrack which bundle to look the value up
			// in; without it the value is ambiguous and the call is refused.
			"$type": f.Type,
			"value": map[string]string{f.key: value},
		}},
	})
}

func (c *Client) post(ctx context.Context, path string, body any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	endpoint := c.base + "/api" + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		// Same unwrap as get: url.Error carries the request URL and buries the
		// cause. It never carries headers, so the token cannot leak either way.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("POST %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return &APIError{Status: resp.StatusCode, Body: readSnippet(resp.Body)}
	}
	return nil
}
