// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package search

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// AgentSearchResult mirrors §8.2 AgentSearchResult. We use json.RawMessage for
// typed_meta because its shape varies by entity_type and the CLI doesn't yet
// need to introspect it.
type AgentSearchResult struct {
	XMLName      xml.Name                     `json:"-" xml:"result"`
	EntityType   string                       `json:"entity_type,omitempty"  xml:"entity_type,omitempty"`
	Title        string                       `json:"title,omitempty"        xml:"title,omitempty"`
	URL          string                       `json:"url,omitempty"          xml:"url,omitempty"`
	Snippet      string                       `json:"snippet,omitempty"      xml:"snippet,omitempty"`
	Score        float64                      `json:"score,omitempty"        xml:"score,omitempty"`
	Source       AgentSearchSource            `json:"source,omitempty"       xml:"source,omitempty"`
	TypedMeta    UnifiedSearchTypedEntityMeta `json:"typed_meta,omitempty"   xml:"-"`
	Extra        map[string]string            `json:"extra,omitempty"        xml:"-"`
	SubType      string                       `json:"sub_type,omitempty"     xml:"sub_type,omitempty"`
	CreateTime   int64                        `json:"create_time,omitempty"  xml:"create_time,omitempty"`
	UpdateTime   int64                        `json:"update_time,omitempty"  xml:"update_time,omitempty"`
	ActivityInfo string                       `json:"activity_info,omitempty" xml:"activity_info,omitempty"`
	ID           string                       `json:"id,omitempty"           xml:"id,omitempty"`
}

// AgentSearchEnvelope is the response wrapper. Truncated vs SizeCapped split per
// §8.2 to disambiguate "too broad" from "you asked for too many".
type AgentSearchEnvelope struct {
	XMLName    xml.Name            `json:"-"                       xml:"envelope"`
	Results    []AgentSearchResult `json:"results,omitempty"       xml:"results>result,omitempty"`
	Warnings   []string            `json:"warnings,omitempty"      xml:"warnings>warning,omitempty"`
	Truncated  bool                `json:"truncated,omitempty"     xml:"truncated,attr,omitempty"`
	SizeCapped bool                `json:"size_capped,omitempty"   xml:"size_capped,attr,omitempty"`
}

// AgentSearchResponse is the top-level RPC return; faas is expected to return
// the same shape verbatim.
type AgentSearchResponse struct {
	Envelope AgentSearchEnvelope `json:"envelope"`
}

// decodeEnvelope handles two response shapes: the canonical
// `{"envelope":{...}}` and the bare envelope that some mock gateways return.
func decodeEnvelope(raw []byte) (AgentSearchEnvelope, error) {
	if len(raw) == 0 {
		return AgentSearchEnvelope{}, nil
	}
	var wrapped struct {
		Envelope *AgentSearchEnvelope `json:"envelope"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Envelope != nil {
		return *wrapped.Envelope, nil
	}
	var bare AgentSearchEnvelope
	if err := json.Unmarshal(raw, &bare); err != nil {
		return AgentSearchEnvelope{}, fmt.Errorf("decode envelope: %w", err)
	}
	return bare, nil
}

// writeEnvelopeText emits a compact human-readable rendering for --format text
// / pretty. One result per block; truncated/size_capped flags surfaced.
func writeEnvelopeText(w io.Writer, env AgentSearchEnvelope) {
	if len(env.Results) == 0 {
		fmt.Fprintln(w, "(no results)")
	}
	for i, r := range env.Results {
		fmt.Fprintf(w, "%d. [%s", i+1, r.EntityType)
		if r.SubType != "" {
			fmt.Fprintf(w, "/%s", r.SubType)
		}
		fmt.Fprint(w, "] ")
		if r.Title != "" {
			fmt.Fprint(w, r.Title)
		} else {
			fmt.Fprint(w, "(no title)")
		}
		if r.Source == SourceLarkDelegated {
			fmt.Fprint(w, "  [delegated]")
		}
		fmt.Fprintln(w)
		if r.URL != "" {
			fmt.Fprintf(w, "   url: %s\n", r.URL)
		}
		if r.ActivityInfo != "" {
			fmt.Fprintf(w, "   activity: %s\n", r.ActivityInfo)
		}
		if r.Snippet != "" {
			fmt.Fprintf(w, "   snippet: %s\n", oneline(r.Snippet, 200))
		}
		if r.CreateTime > 0 {
			fmt.Fprintf(w, "   create_time: %s\n", time.Unix(r.CreateTime, 0).Format(time.RFC3339))
		}
	}
	if env.SizeCapped {
		fmt.Fprintln(w, "(warning: --size exceeded server cap; clamped to 100)")
	}
	if env.Truncated {
		fmt.Fprintln(w, "(warning: results truncated; narrow the query or add filters)")
	}
	for _, warn := range env.Warnings {
		fmt.Fprintf(w, "(warning: %s)\n", warn)
	}
}

// MarshalXML emits AgentSearchResult so that typed_meta survives the XML
// round-trip. We can't rely on the default encoding/xml reflection because
// typed_meta is `json.RawMessage` (server-side typed bytes the CLI doesn't
// introspect) and `extra` is a map — neither marshals through stdlib XML by
// default. Wrapping typed_meta in <![CDATA[...]]> keeps the original JSON bytes
// verbatim so LLM consumers see the same fields they'd see in --format json
// (calendar event organizer, chat_id, sender_id, sub_type details, etc.). extra
// is still dropped — it's a free-form map only emitted by a handful of entities
// and not currently load-bearing.
func (r AgentSearchResult) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Name = xml.Name{Local: "result"}
	if err := e.EncodeToken(start); err != nil {
		return err
	}
	writeStr := func(tag, val string) error {
		if val == "" {
			return nil
		}
		return e.EncodeElement(val, xml.StartElement{Name: xml.Name{Local: tag}})
	}
	if err := writeStr("entity_type", r.EntityType); err != nil {
		return err
	}
	if err := writeStr("title", r.Title); err != nil {
		return err
	}
	if err := writeStr("url", r.URL); err != nil {
		return err
	}
	if err := writeStr("snippet", r.Snippet); err != nil {
		return err
	}
	if r.Score != 0 {
		if err := e.EncodeElement(r.Score, xml.StartElement{Name: xml.Name{Local: "score"}}); err != nil {
			return err
		}
	}
	if r.Source != 0 {
		if err := e.EncodeElement(int(r.Source), xml.StartElement{Name: xml.Name{Local: "source"}}); err != nil {
			return err
		}
	}
	if len(r.TypedMeta) > 0 {
		type cdataWrap struct {
			V string `xml:",cdata"`
		}
		if err := e.EncodeElement(cdataWrap{V: string(r.TypedMeta)}, xml.StartElement{Name: xml.Name{Local: "typed_meta"}}); err != nil {
			return err
		}
	}
	if err := writeStr("sub_type", r.SubType); err != nil {
		return err
	}
	if r.CreateTime > 0 {
		if err := e.EncodeElement(r.CreateTime, xml.StartElement{Name: xml.Name{Local: "create_time"}}); err != nil {
			return err
		}
	}
	if r.UpdateTime > 0 {
		if err := e.EncodeElement(r.UpdateTime, xml.StartElement{Name: xml.Name{Local: "update_time"}}); err != nil {
			return err
		}
	}
	if err := writeStr("activity_info", r.ActivityInfo); err != nil {
		return err
	}
	if err := writeStr("id", r.ID); err != nil {
		return err
	}
	return e.EncodeToken(xml.EndElement{Name: start.Name})
}

// writeEnvelopeXML emits the envelope as indented XML. Intended for agents that
// pipe results through XML-shaped prompts. typed_meta is preserved via the
// custom AgentSearchResult.MarshalXML above; extra is still dropped.
func writeEnvelopeXML(w io.Writer, env AgentSearchEnvelope) error {
	if _, err := fmt.Fprintln(w, `<?xml version="1.0" encoding="UTF-8"?>`); err != nil {
		return err
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(env); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w)
	return err
}

// emitEnvelope is the shared output sink. It intercepts `--format xml` (which
// the upstream framework would otherwise warn-and-fall-back-to-json on) and
// writes XML straight to stdout; for any other format it hands off to
// runtime.OutFormat with writeEnvelopeText as the pretty renderer.
func emitEnvelope(runtime *common.RuntimeContext, env AgentSearchEnvelope) {
	if strings.EqualFold(strings.TrimSpace(runtime.Format), "xml") {
		_ = writeEnvelopeXML(runtime.IO().Out, env)
		return
	}
	runtime.OutFormat(env, &output.Meta{Count: len(env.Results)}, func(w io.Writer) {
		writeEnvelopeText(w, env)
	})
}

// oneline collapses whitespace and truncates so each snippet stays on a single
// terminal row.
func oneline(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}
