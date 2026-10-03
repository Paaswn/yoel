package core

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	gapi "github.com/Paaswn/yoel/graderapi"
	_ "modernc.org/sqlite"
)

type RegistryMetadata struct {
    LastUpdate time.Time `json:"last_update"`
}
type Registry struct {
    db *sql.DB
}

var (
    ProblemNotFound = sql.ErrNoRows
)
const yoelCache = "yoel"
const yoelDatabase = "yoel.db"
func yoelNormalDBPath() ( string, error ) {
    configDir, err := os.UserConfigDir()
    if err != nil {
        return "", err
    }
    dirPath := filepath.Join(configDir, yoelCache)

    if err := os.MkdirAll(dirPath, 0700); err != nil {
        return "", err
    }
    dbPath := filepath.Join(dirPath, yoelDatabase)
    return dbPath, nil
}
func newRegistry(dbPath string) ( *Registry, error) {
    db, err := sql.Open("sqlite", dbPath)
    if err != nil {
        return nil, err
    }
    if err := db.Ping(); err != nil {
        db.Close()
        return nil, err
    }

    db.SetMaxOpenConns(1)
    r := &Registry{db: db}
    if err := r.init(); err != nil {
        db.Close()
        return nil, err
    }
	return r, nil
}
func NewRegistry() ( *Registry, error) {
    dbPath, err := yoelNormalDBPath()
    if err != nil {
        return nil, err
    }
    r, err := newRegistry(dbPath)
    if err != nil {
        return nil, err
    }
	return r, nil
}

type problemLite struct {
    id int
    has_attachment bool
    sort_order int
    best_score sql.NullFloat64
    code_name string
    pretty_name string
    source_path sql.NullString // source path indicates file that will be sent to grader such as student.h or main.cpp
    directory_path sql.NullString
}

type ProblemLite struct {
    ID int
    HasAttachment bool
    SortOrder int
    BestScore float64
    CodeName string
    PrettyName string
    SourcePath string // source path indicates file that will be sent to grader such as student.h or main.cpp
    DirectoryPath string
    IsOnLocal bool
}
func (r *Registry) init() error {
    _, err := r.db.Exec(`
        CREATE TABLE IF NOT EXISTS problems (
        id INTEGER PRIMARY KEY,
        has_attachment INTEGER NOT NULL DEFAULT 0,
        sort_order INTEGER NOT NULL,
        best_score REAL,
        code_name TEXT NOT NULL,
        pretty_name TEXT NOT NULL,
        source_path TEXT,
        directory_path TEXT
        )
    `)
    return err
}


func (r *Registry) Upsert(p ProblemLite) error {
    _, err := r.db.Exec(`
        INSERT INTO problems (id, has_attachment, sort_order, best_score, code_name, pretty_name)
        VALUES (?, ?, ?, ?, ?, ?)
        ON CONFLICT(id) DO UPDATE SET
            has_attachment = excluded.has_attachment,
            sort_order = excluded.sort_order,
            best_score = excluded.best_score,
            code_name = excluded.code_name,
            pretty_name = excluded.pretty_name
        `,
        p.ID,
        p.HasAttachment,
        p.SortOrder,
        p.BestScore,
        p.CodeName,
        p.PrettyName,
    )
    return err
}

func (r *Registry) SetProblemPath(id int, sourcePath string, directoryPath string) error {
    _, err := r.db.Exec(`
        UPDATE problems
        SET source_path = ?,
            directory_path = ?
        WHERE id = ?
    `,
        sourcePath,
        directoryPath,
        id,
    )
    return err
}

func (r *Registry) UpdateProblemScore(id int, score float64) error {
    _, err := r.db.Exec(`
        UPDATE problems
        SET best_score = ?
        WHERE id = ?
    `,
        score,
        id,
    )
    return err
}
// since id is unique this method give you exactly one row, hence I use QueryRow here
func (r *Registry) QueryByID(id int) (ProblemLite, error ) {
    var p problemLite
    err := r.db.QueryRow(`
        SELECT id, has_attachment, best_score, code_name, pretty_name, source_path, directory_path
        FROM problems
        WHERE id = ?
    `, id).Scan(&p.id, &p.has_attachment,  &p.best_score, &p.code_name, &p.pretty_name, &p.source_path, &p.directory_path)
    if err != nil {
        return ProblemLite{}, err
    }
    return ProblemLite{
        ID: p.id,
        HasAttachment: p.has_attachment,
        BestScore: p.best_score.Float64,
        CodeName: p.code_name,
        PrettyName: p.pretty_name,
        SourcePath: p.source_path.String,
        DirectoryPath: p.directory_path.String,
        IsOnLocal: p.directory_path.Valid,
    }, nil
}

func (r *Registry) QueryByName(name string) ([]ProblemLite, error) {
    query := "%"+name+"%"
    rows, err := r.db.Query(`
        SELECT  id,
                has_attachment,
                best_score,
                code_name,
                pretty_name,
                source_path,
                directory_path
        FROM problems
        WHERE code_name LIKE ?
            OR pretty_name LIKE ?
        ORDER BY sort_order ASC;
    `, query, query)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var result []ProblemLite
    for rows.Next() {
        var p problemLite
        if err := rows.Scan(
            &p.id,
            &p.has_attachment,
            &p.best_score,
            &p.code_name,
            &p.pretty_name,
            &p.source_path,
            &p.directory_path,
        ); err != nil {
            return nil, err
        }

        result = append(result, ProblemLite{
            ID:            p.id,
            HasAttachment: p.has_attachment,
            BestScore:     p.best_score.Float64,
            CodeName:      p.code_name,
            PrettyName:    p.pretty_name,
            SourcePath:    p.source_path.String,
            DirectoryPath: p.directory_path.String,
            IsOnLocal:     p.directory_path.Valid,
        })
    }
    if err := rows.Err(); err != nil {
        return nil, err
    }
    if len(result) == 0 {
        return nil, ProblemNotFound
    }
    return result, nil
}

func (r *Registry) updateAllQuestions(ctx context.Context, client *gapi.Client) error {
    questions, err := client.ListProblems(ctx)
    if err != nil {
        return err
    }
    for i, q := range questions {
        var bestScore float64
        if q.BestScore == nil {
            bestScore = 0
        } else {
            bestScore = *q.BestScore
        }
        p := ProblemLite{
            ID: q.ID,
            HasAttachment: q.HasAttachment,
            SortOrder: i,
            BestScore: bestScore,
            CodeName: q.CodeName,
            PrettyName: q.PrettyName,
        }
        if err := r.Upsert(p); err != nil {
            return err
        }

    }
    return nil
}
func (r *Registry) UpdateAllQuestions(ctx context.Context, session SavedSession) error {
    client, err := gapi.NewClient(DefaultGraderURL, nil )
    if err != nil {
        return err
    }
    ctx, cancel := context.WithTimeout(ctx, TimeOut)
    defer cancel()
    return r.updateAllQuestions(ctx, client.WithToken(session.Token))
}

func (r *Registry) GetAllQuestions(session SavedSession) ([]ProblemLite, error ) {

    rows, err := r.db.Query(`
            SELECT
                id,
                has_attachment,
                best_score,
                code_name,
                pretty_name,
                source_path,
                directory_path
            FROM problems
            ORDER BY sort_order ASC;
        `)
        if err != nil {
            return nil, err
        }
        defer rows.Close()

        problems := make([]ProblemLite, 0)

        for rows.Next() {
            var p problemLite

            if err := rows.Scan(
                &p.id,
                &p.has_attachment,
                &p.best_score,
                &p.code_name,
                &p.pretty_name,
                &p.source_path,
                &p.directory_path,
            ); err != nil {
                return nil, err
            }

            problems = append(problems, ProblemLite{
                ID:            p.id,
                BestScore:     p.best_score.Float64,
                CodeName:      p.code_name,
                PrettyName:    p.pretty_name,
                SourcePath:    p.source_path.String,
                DirectoryPath: p.directory_path.String,
                IsOnLocal:     p.directory_path.Valid,
            })
        }
        if err := rows.Err(); err != nil {
            return nil, err
        }
        return problems, nil
}
func (r *Registry) Close() error {
    return r.db.Close()
}

func ProblemNotFoundNotice(w io.Writer) error{
    fmt.Fprintln(w, "Registry may be outdated. Try running 'yoel fetch' to update the registry")
    return ProblemNotFound
}
