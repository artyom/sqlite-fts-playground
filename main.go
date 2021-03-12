// TODO describe program
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"strings"

	_ "modernc.org/sqlite"
)

func main() {
	log.SetFlags(0)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	args := runArgs{Database: "docs-index.db"}
	flag.StringVar(&args.IndexDir, "index", args.IndexDir, "index .md documents in this directory")
	flag.StringVar(&args.Database, "db", args.Database, "index file")
	flag.Parse()
	if err := run(ctx, args, flag.Args()...); err != nil {
		log.Fatal(err)
	}
}

type runArgs struct {
	IndexDir string
	Database string
}

func run(ctx context.Context, args runArgs, query ...string) error {
	if args.Database == "" {
		return errors.New("-db must be set")
	}
	db, err := sql.Open("sqlite", args.Database)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, `create virtual table if not exists files using fts5(filename, body)`)
	if err != nil {
		return err
	}
	if args.IndexDir != "" {
		return indexDir(ctx, db, args.IndexDir)
	}
	if len(query) == 0 {
		return errors.New("nothing to search ")
	}
	return search(ctx, db, os.Stdout, strings.Join(query, " "))
}

func search(ctx context.Context, db *sql.DB, w io.Writer, query string) error {
	rows, err := db.QueryContext(ctx, `select filename, snippet(files, 1, '<mark>', '</mark>', '...', 5)
		from files where files match ?`, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var filename, snippet string
		if err := rows.Scan(&filename, &snippet); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "File: %s\n\n%s\n\n", filename, snippet); err != nil {
			return err
		}
	}
	return rows.Err()
}

func indexDir(ctx context.Context, db *sql.DB, dir string) error {
	dirFS := os.DirFS(dir)
	fn := func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return fs.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".md") {
			return nil
		}
		body, err := fs.ReadFile(dirFS, p)
		if err != nil {
			return err
		}
		_, err = db.ExecContext(ctx, `insert into files (filename, body) values (?, ?)`, p, body)
		return err
	}
	return fs.WalkDir(dirFS, ".", fn)
}
