package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
	"gosuda.org/website/internal/markdown"
	"gosuda.org/website/internal/types"
)

const (
	rootDir   = "root"
	publicDir = "public"
	distDir   = "dist"
	dbFile    = "zdata/data.json.zstd"
)

var (
	ErrInvalidMarkdown = fmt.Errorf("invalid markdown file")
)

//go:generate templ generate
//go:generate bun run build

func generateFileList(dir string) ([]string, error) {
	var fileList []string
	err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if !info.IsDir() {
			fileList = append(fileList, path)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return fileList, nil
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = srcFile.WriteTo(dstFile)
	if err != nil {
		return err
	}
	return nil
}

func generatePath(title string) string {
	fp := strings.TrimPrefix(title, rootDir)
	for strings.HasPrefix(fp, "/") {
		fp = strings.TrimPrefix(fp, "/")
	}

	fp = strings.ToLower(fp)
	fp = strings.ReplaceAll(fp, " ", "-")
	fp = strings.ReplaceAll(fp, "/", "-")
	fp = strings.ReplaceAll(fp, `{`, "-")
	fp = strings.ReplaceAll(fp, `}`, "-")
	fp = strings.ReplaceAll(fp, `|`, "-")
	fp = strings.ReplaceAll(fp, `\`, "-")
	fp = strings.ReplaceAll(fp, `^`, "-")
	fp = strings.ReplaceAll(fp, `~`, "-")
	fp = strings.ReplaceAll(fp, `[`, "-")
	fp = strings.ReplaceAll(fp, `]`, "-")
	fp = strings.ReplaceAll(fp, `'`, "-")
	fp = strings.ReplaceAll(fp, `"`, "-")
	fp = strings.ReplaceAll(fp, "`", "-")

	fp = strings.ReplaceAll(fp, `----`, "-")
	fp = strings.ReplaceAll(fp, `---`, "-")
	fp = strings.ReplaceAll(fp, `--`, "-")
	fp = strings.ReplaceAll(fp, `--`, "-")
	fp = strings.ReplaceAll(fp, `--`, "-")

	var b [4]byte
	rand.Read(b[:])
	fp = fmt.Sprintf("/blog/posts/%s-%x", fp, b)

	return fp
}

func copyDir(src, dst string) error {
	filepath.Walk(src, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath := strings.TrimPrefix(path, src)
		dstPath := filepath.Join(dst, relPath)
		if info.IsDir() {
			err := os.MkdirAll(dstPath, os.ModePerm)
			if err != nil {
				return err
			}
		} else {
			err := copyFile(path, dstPath)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return nil
}

// parseMarkdown renders the given markdown data into HTML.
func parseMarkdown(path string, data []byte) (*types.Document, error) {
	log.Debug().Str("path", path).Msgf("rendering markdown file %s", path)
	doc, err := markdown.ParseMarkdown(string(data))
	if err != nil {
		return nil, err
	}
	log.Debug().Str("path", path).Int("rendered_size", len(doc.HTML)).Msgf("rendered markdown file %s", path)
	return doc, nil
}

// processMarkdownFile processes a markdown file and returns the rendered HTML document.
func processMarkdownFile(gc *GenerationContext, path string) (*types.Document, error) {
	log.Debug().Str("path", path).Msgf("start processing markdown file %s", path)

	log.Debug().Str("path", path).Msgf("start reading markdown file %s", path)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	log.Debug().Str("path", path).Int("size", len(data)).Msgf("read markdown file %s", path)

	doc, err := parseMarkdown(path, data)
	if err != nil {
		return nil, err
	}

	var updated bool

	if doc.Metadata.ID == "" {
		updated = true
		doc.Metadata.ID = types.RandID()
		log.Debug().Str("path", path).Str("id", doc.Metadata.ID).Msgf("assigned new ID to document %s", path)
	}

	if doc.Metadata.Date.IsZero() {
		updated = true
		doc.Metadata.Date = time.Now().UTC()
		log.Debug().Str("path", path).Msgf("assigned new date to document %s", path)
	}

	if doc.Metadata.Path == "" {
		updated = true
		doc.Metadata.Path = generatePath(doc.Metadata.Title)
	}

	if updated {
		log.Debug().Str("path", path).Msgf("saving updated document %s", path)

		if doc.Type == types.DocumentTypeMarkdown {
			newMeta, err := yaml.Marshal(&doc.Metadata)
			if err != nil {
				return nil, err
			}

			original := doc.Markdown
			original = strings.TrimPrefix(original, "---\n")
			_, origDocument, ok := strings.Cut(original, "---\n")
			if !ok {
				return nil, ErrInvalidMarkdown
			}
			newDocument := "---\n" + string(newMeta) + "---\n" + origDocument
			doc.Markdown = newDocument

			fStat, err := os.Stat(path)
			if err != nil {
				return nil, err
			}

			err = os.WriteFile(path, []byte(doc.Markdown), fStat.Mode())
			if err != nil {
				return nil, err
			}
			log.Debug().Str("path", path).Msgf("saved updated document %s", path)
		} else {
			log.Debug().Str("path", path).Msgf("skipping non-markdown document %s", path)
		}
	}

	now := time.Now()

	// Update Post Object
	var post *types.Post
	if p, ok := gc.DataStore.Posts[doc.Metadata.ID]; ok {
		post = p
	} else {
		post = &types.Post{
			ID:         doc.Metadata.ID,
			CreatedAt:  now,
			UpdatedAt:  now,
			Translated: make(map[string]*types.Document),
		}
		gc.DataStore.Posts[doc.Metadata.ID] = post
	}

	hash := doc.Hash()
	post.FilePath = path
	post.Path = doc.Metadata.Path
	post.Main = doc
	if post.Hash != hash {
		post.Hash = hash
		post.UpdatedAt = now
	}

	log.Debug().Str("path", path).Msgf("end processing markdown file %s", path)
	return doc, nil
}

func generate(gc *GenerationContext) error {
	log.Debug().Msg("start generating website")

	distInfo, err := os.Stat(distDir)
	if err == nil && distInfo.IsDir() {
		log.Debug().Msg("deleting dist directory")
		err := os.RemoveAll(distDir)
		if err != nil {
			return err
		}
		log.Debug().Msg("deleted dist directory")
	}

	log.Debug().Msg("copying static files")
	err = copyDir(publicDir, distDir)
	if err != nil {
		return err
	}
	log.Debug().Msg("copied static files")

	log.Debug().Msg("creating root file index")
	list, err := generateFileList(rootDir)
	if err != nil {
		return err
	}

	for _, path := range list {
		log.Debug().Str("path", path).Msgf("processing file %s", path)
		switch strings.ToLower(filepath.Ext(path)) {
		case ".md", ".markdown":
			_, err := processMarkdownFile(gc, path)
			if err != nil {
				log.Error().Err(err).Str("path", path).Msgf("failed to process markdown file %s", path)
			}
		default:
			log.Debug().Str("path", path).Msgf("skipping %s", path)
		}
		log.Debug().Str("path", path).Msgf("processed file %s", path)
	}

	log.Debug().Msg("end generating website")
	return nil
}

func main() {
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "2006-01-02 15:04:05"})

	_, err := os.Stat(dbFile)
	if err != nil && !os.IsNotExist(err) {
		log.Fatal().Err(err).Msgf("failed to stat database file %s", dbFile)
	}

	var f *os.File
	if err != nil && os.IsNotExist(err) {
		log.Info().Err(err).Msgf("database file %s does not exist, Creating new database file", dbFile)
		f, err = os.OpenFile(dbFile, os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			log.Fatal().Err(err).Msgf("failed to create database file %s", dbFile)
		}

		w, err := zstd.NewWriter(f)
		if err != nil {
			log.Fatal().Err(err).Msgf("failed to create zstd writer for database file %s", dbFile)
		}

		_, err = w.Write([]byte("{}"))
		if err != nil {
			log.Fatal().Err(err).Msgf("failed to write to database file %s", dbFile)
		}

		err = w.Close()
		if err != nil {
			log.Fatal().Err(err).Msgf("failed to close zstd writer for database file %s", dbFile)
		}

		_, err = f.Seek(0, 0)
		if err != nil {
			log.Fatal().Err(err).Msgf("failed to seek to beginning of database file %s", dbFile)
		}
	} else {
		f, err = os.OpenFile(dbFile, os.O_RDWR, 0644)
		if err != nil {
			log.Fatal().Err(err).Msgf("failed to open database file %s", dbFile)
		}
	}

	var gc GenerationContext

	r, err := zstd.NewReader(f)
	if err != nil {
		log.Fatal().Err(err).Msgf("failed to create zstd reader for database file %s", dbFile)
	}

	err = json.NewDecoder(r).Decode(&gc.DataStore)
	if err != nil {
		log.Fatal().Err(err).Msgf("failed to decode database file %s", dbFile)
	}
	r.Close()

	err = f.Close()
	if err != nil {
		log.Fatal().Err(err).Msgf("failed to close database file %s", dbFile)
	}

	if gc.DataStore == nil {
		gc.DataStore = &DataStore{}
	}

	if gc.DataStore.Posts == nil {
		gc.DataStore.Posts = make(map[string]*types.Post)
	}

	generate(&gc)

	// Update Database
	f, err = os.OpenFile(dbFile+".tmp", os.O_CREATE|os.O_RDWR|os.O_TRUNC|os.O_EXCL, 0644)
	if err != nil {
		log.Fatal().Err(err).Msgf("failed to create temporary database file %s", dbFile)
	}

	w, err := zstd.NewWriter(f, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
	if err != nil {
		log.Fatal().Err(err).Msgf("failed to create zstd writer for database file %s", dbFile)
	}

	err = json.NewEncoder(w).Encode(gc.DataStore)
	if err != nil {
		log.Fatal().Err(err).Msgf("failed to encode database file %s", dbFile)
	}

	err = w.Close()
	if err != nil {
		log.Fatal().Err(err).Msgf("failed to close zstd writer for database file %s", dbFile)
	}

	err = f.Close()
	if err != nil {
		log.Fatal().Err(err).Msgf("failed to close database file %s", dbFile)
	}

	err = os.Rename(dbFile+".tmp", dbFile)
	if err != nil {
		log.Fatal().Err(err).Msgf("failed to rename temporary database file %s", dbFile)
	}

	log.Info().Msgf("database file %s updated", dbFile)
	log.Info().Msgf("website generated")

	// print database as JSON
	jsonData, err := json.MarshalIndent(gc.DataStore, "", "  ")
	if err != nil {
		log.Fatal().Err(err).Msgf("failed to marshal database file %s", dbFile)
	}
	fmt.Println(string(jsonData))
}
