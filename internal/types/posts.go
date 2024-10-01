package types

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/zeebo/blake3"
)

func RandID() string {
	var b [16]byte
	_, err := rand.Read(b[:])
	if err != nil {
		panic("failed to generate random ID")
	}
	return hex.EncodeToString(b[:])
}

// Post represents a blog post or similar content item.
type Post struct {
	// ID is the unique identifier for the post.
	ID string `json:"id" yaml:"id"`
	// FilePath is the file path to the post file.
	FilePath string `json:"file_path" yaml:"file_path"`
	// Path is the URL path for the post.
	Path string `json:"path" yaml:"path"`
	// Hash is a hash of the raw content to detect changes.
	Hash string `json:"hash" yaml:"hash"`

	// CreatedAt is the date and time when the post was created.
	CreatedAt time.Time `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	// UpdatedAt is the date and time when the post was last updated.
	UpdatedAt time.Time `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`

	// Main contains the primary language version of the post content.
	Main *Document `json:"main,omitempty" yaml:"main,omitempty"`
	// Translated contains translated versions of the post content, keyed by language code.
	Translated map[string]*Document `json:"translated,omitempty" yaml:"translated,omitempty"`
}

// DocumentType represents the type of a document (e.g., Markdown, HTML).
//
//go:generate stringer -type=DocumentType -output=post_types.go -linecomment
type DocumentType uint16

const (
	DocumentTypeUnknown  DocumentType = iota // unknown
	DocumentTypeMarkdown                     // markdown
	DocumentTypeHTML                         // html
)

// Document represents the content and metadata of a post in a specific language.
type Document struct {
	// Type indicates the format of the document content.
	Type DocumentType `json:"type" yaml:"type"`
	// Markdown is the raw Markdown content of the document (if applicable).
	Markdown string `json:"markdown,omitempty" yaml:"markdown,omitempty"`
	// HTML is the HTML content of the document, either directly provided or rendered from Markdown.
	HTML string `json:"html,omitempty" yaml:"html,omitempty"`
	// Metadata contains any additional metadata parsed from the Markdown document.
	Metadata Metadata `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// Metadata is a struct that holds various types of meta data parsed from a Markdown document
type Metadata struct {
	// ID is the unique identifier for the post. (propagated to Post.ID)
	ID string `json:"id" yaml:"id"`
	// Title is the title of the document.
	Title string `json:"title,omitempty" yaml:"title,omitempty"`
	// Author is the author of the document.
	Author string `json:"author,omitempty" yaml:"author,omitempty"`
	// Description is a brief description of the document.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// Date is the publication date of the document.
	Date time.Time `json:"date,omitempty" yaml:"date,omitempty"`
	// Path is the URL path for the post. (propagated to Post.Path)
	Path string `json:"path,omitempty" yaml:"path,omitempty"`
	// GoPackage is the Go package associated with the post (optional). Only effective if the post is Main Document.
	GoPackage string `json:"go_package,omitempty" yaml:"go_package,omitempty"`
	// Canonical is the canonical URL for the post.
	Canonical string `json:"canonical,omitempty" yaml:"canonical,omitempty"`
	// Hidden indicates whether the post should be listed on the front page.
	Hidden bool `json:"hidden,omitempty" yaml:"hidden,omitempty"`
}

func (g *Metadata) Hash() string {
	h := blake3.New()
	h.Write([]byte(g.ID))
	h.WriteString(g.Title)
	h.WriteString(g.Author)
	h.WriteString(g.Description)
	h.WriteString(g.Date.Format(time.RFC3339))
	h.WriteString(g.Path)
	h.WriteString(g.GoPackage)
	h.WriteString(g.Canonical)
	h.WriteString(strconv.FormatBool(g.Hidden))
	return hex.EncodeToString(h.Sum(nil))
}

func (g *Document) Hash() string {
	h := blake3.New()
	h.WriteString(g.Type.String())
	h.WriteString(g.Markdown)
	h.WriteString(g.HTML)
	h.WriteString(g.Metadata.Hash())
	return hex.EncodeToString(h.Sum(nil))
}
