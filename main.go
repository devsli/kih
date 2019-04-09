package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"text/template"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/nandosousafr/podfeed"
)

type episodeRow struct {
	pubdate     string
	size        uint64
	title       string
	author      string
	subtitle    string
	summary     string
	image       string
	url         string
	mimeType    string
	guid        string
	description string
	duration    string
	explicit    string
}

const castURL = "http://78.140.251.40/tmp_audio/itunes2/hik_-_rr_%s.mp3"
const sqlCreateTable = `CREATE TABLE IF NOT EXISTS episodes
	(pubdate,
	len integer,
	title,
	author,
	subtitle,
	summary,
	image,
	url PRIMARY KEY,
	type,
	guid,
	description,
	duration,
	explicit);`

const sqlInsert = `INSERT OR REPLACE INTO episodes (pubdate,
	len,
	title,
	author,
	subtitle,
	summary,
	image,
	url,
	type,
	guid,
	description,
	duration,
	explicit) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

var db *sql.DB
var inserter *sql.Stmt

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func saveItem(item podfeed.Item) (sql.Result, error) {
	return inserter.Exec(
		item.PubDate.Value,
		item.Enclosure.Length,
		item.Title,
		item.Author,
		item.Subtitle,
		item.Summary,
		item.Image.Href,
		item.Enclosure.Url,
		item.Enclosure.Type,
		item.Enclosure.Url,
		item.Description,
		item.Duration,
		"clean")
}

func sync() {
	podcast, err := podfeed.Fetch("http://www.radiorecord.ru/rss.xml")
	check(err)

	for _, item := range podcast.Items {
		if strings.Contains(item.Enclosure.Url, "itunes2/hik_-_rr") {
			saveItem(item)
		}
	}
}

func responseToPodfeed(url string, resp *http.Response) podfeed.Item {
	var pubDate, _ = time.Parse(time.RFC1123, resp.Header["Last-Modified"][0])
	var castDate, _ = time.Parse("2006-01-02", url[len(url)-14:len(url)-4])

	return podfeed.Item{
		Title:   fmt.Sprintf("Кремов и Хрусталев @ Radio Record (%s)", castDate.Format("02-01-2006")),
		PubDate: podfeed.Time{Value: pubDate},
		Link:    url,
		Author:  "Radio Record",

		Enclosure: podfeed.Enclosure{
			Length: uint64(resp.ContentLength),
			Url:    url,
			Type:   resp.Header["Content-Type"][0]},

		Image: podfeed.Image{
			Href: "http://www.radiorecord.ru/upload/iblock/0dd/0ddd4f32b459515dde4ad7e7b5e5fd30.jpg"}}
}

func sqlDate(str string) time.Time {
	result, err := time.Parse(time.RFC3339, strings.Replace(str, " ", "T", 1))
	check(err)
	return result
}

func lastEpisodeDate() time.Time {
	var lastSeen string
	row := db.QueryRow("SELECT MAX(pubdate) FROM episodes")
	row.Scan(&lastSeen)
	if lastSeen == "" {
		return time.Date(2018, time.January, 8, 0, 0, 0, 0, time.UTC)
	}
	return sqlDate(lastSeen)
}

func loadHistory() {
	var now = time.Now()
	var startDate = lastEpisodeDate()

	for castDate := startDate; castDate.Before(now); castDate = castDate.AddDate(0, 0, 1) {
		url := fmt.Sprintf(castURL, castDate.Format("2006-01-02"))
		result, _ := http.Head(url)

		if result.StatusCode != 404 {
			_, err := saveItem(responseToPodfeed(url, result))
			check(err)
			log.Printf("[+] %s", url)
		}
	}
}

func prune() {
	var (
		url  string
		rows *sql.Rows
	)

	rows, _ = db.Query("SELECT url FROM episodes ORDER BY pubdate DESC")
	defer rows.Close()
	for rows.Next() {
		if err := rows.Scan(&url); err != nil {
			log.Fatal(err)
		}

		result, _ := http.Head(url)

		if result.StatusCode == 404 {
			db.Exec("DELETE FROM episodes WHERE url = ?", url)
			log.Printf("[-] %s", url)
		}
	}
}

func usage() {
	println("Usage:")
	println("kih [fetch|sync|help]")
}

func rowToItem(e episodeRow) podfeed.Item {
	return podfeed.Item{
		Title:    e.title,
		PubDate:  podfeed.Time{Value: sqlDate(e.pubdate)},
		Link:     e.url,
		Author:   e.author,
		Image:    podfeed.Image{Href: e.image},
		Duration: e.duration,

		Enclosure: podfeed.Enclosure{
			Length: e.size,
			Url:    e.url,
			Type:   e.mimeType}}
}

func makeRSS(podcast podfeed.Podcast) {
	var (
		e    episodeRow
		rows *sql.Rows
	)

	rows, _ = db.Query(`SELECT
		pubdate, len, title, author, subtitle, summary, image, url, type, guid,
		description, duration, explicit
	  FROM episodes ORDER BY pubdate DESC`)
	defer rows.Close()

	for rows.Next() {
		if err := rows.Scan(&e.pubdate, &e.size, &e.title, &e.author,
			&e.subtitle, &e.summary, &e.image, &e.url, &e.mimeType, &e.guid,
			&e.description, &e.duration, &e.explicit); err != nil {
			log.Fatal(err)
		}

		podcast.Items = append(podcast.Items, rowToItem(e))
	}

	t, err := template.New("rss.xml.tmpl").ParseFiles("rss.xml.tmpl")
	check(err)
	err = t.Execute(os.Stdout, podcast)
	check(err)
}

func init() {
	db, _ = sql.Open("sqlite3", "./episodes.db")
	db.Exec(sqlCreateTable)
	inserter, _ = db.Prepare(sqlInsert)
}

func main() {
	defer db.Close()

	if len(os.Args) == 1 {
		usage()
		return
	}

	switch os.Args[1] {
	case "fetch":
		loadHistory()
	case "sync":
		sync()
	case "rss":
		fields := podfeed.Podcast{
			Author:      "www.radiorecord.ru",
			Category:    podfeed.Category{Text: "Comedy"},
			Description: "Программы Радио Рекорд",
			Image:       podfeed.Image{Href: "http://www.radiorecord.ru/upload/iblock/0dd/0ddd4f32b459515dde4ad7e7b5e5fd30.jpg"},
			Language:    "ru-ru",
			Link:        "http://kih.suprun.pw",
			Owner:       podfeed.Owner{Name: "Marsel Markhabulin", Email: "milar@marsell.ws"},
			Subtitle:    "Radio Record",
			Title:       "Radio Record / Кремов и Хрусталев"}

		makeRSS(fields)
	case "prune":
		prune()
	case "update":
		prune()
		fallthrough
	case "init":
		loadHistory()
		sync()
	default:
		usage()
	}
}
