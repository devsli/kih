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

const castURL = "http://78.140.251.40/tmp_audio/itunes2/hik_-_rr_%s.mp3"
const rssTpl = `<?xml version="1.0" encoding="UTF-8"?>
<rss xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd" version="2.0"
	 xmlns:atom="http://www.w3.org/2005/Atom"
	 xmlns:googleplay="http://www.google.com/schemas/play-podcasts/1.0">
	<channel>
	<title>Radio Record / Кремов и Хрусталев</title>
	<link>http://www.radiorecord.ru</link>
	<language>ru-ru</language>
	<copyright>Radio Record Russia</copyright>
	<atom:link href="http://www.radiorecord.ru/rss.xml" rel="self" type="application/rss+xml" />
	<itunes:subtitle>Radio Record</itunes:subtitle>
	<itunes:author>www.radiorecord.ru</itunes:author>
	<googleplay:author>www.radiorecord.ru</googleplay:author>
	<itunes:summary>Программы Радио Рекорд</itunes:summary>
	<description>Программы Радио Рекорд</description>
	<itunes:owner>
		<itunes:name>Marsel Markhabulin</itunes:name>
		<itunes:email>milar@marsell.ws</itunes:email>
	</itunes:owner>
	<itunes:image href="http://www.radiorecord.ru/upload/iblock/0dd/0ddd4f32b459515dde4ad7e7b5e5fd30.jpg" />
	<googleplay:image href="http://www.radiorecord.ru/upload/iblock/0dd/0ddd4f32b459515dde4ad7e7b5e5fd30.jpg"/>
	<itunes:category text="Comedy"/>
	<googleplay:category text="Comedy"/>
	<itunes:explicit>clean</itunes:explicit>
	{{ range .Items -}}
	<item>
		<title>{{ .Title }}</title>
		<itunes:author>{{ .Author }}</itunes:author>
		<itunes:subtitle>{{ .Subtitle }}</itunes:subtitle>
		<itunes:summary>{{ .Summary }}</itunes:summary>
		<itunes:image href="{{ .Image.Href }}" />
		<enclosure url="{{ .Enclosure.Url }}" length="{{ .Enclosure.Length }}" type="{{ .Enclosure.Type }}" />
		<guid>{{ .Enclosure.Url }}</guid>
		<pubDate>{{ .PubDate.Value.Format "Mon, 02 Jan 2006 15:04:05 MST" }}</pubDate>
		<description>{{ .Description }}</description>
		<itunes:duration>{{ .Duration }}</itunes:duration>
		<itunes:explicit>clean</itunes:explicit>
	</item>
	{{- end }}
	</channel>
</rss>`

const sqlCreateTable = `CREATE TABLE IF NOT EXISTS episodes
	(pubdate,
	len integer,
	title,
	itunes_author,
	itunes_subtitle,
	itunes_summary,
	itunes_image,
	url PRIMARY KEY,
	type,
	guid,
	description,
	itunes_duration,
	itunes_explicit);`

const sqlInsert = `INSERT OR REPLACE INTO episodes (pubdate,
	len,
	title,
	itunes_author,
	itunes_subtitle,
	itunes_summary,
	itunes_image,
	url,
	type,
	guid,
	description,
	itunes_duration,
	itunes_explicit) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

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

func loadMetadata() {
	/*
		var (
			row *sql.Row
			url string
		)

		row = db.QueryRow("SELECT url FROM episodes WHERE itunes_duration = \"\"")
		row.Scan(&url)
		/*
			out, _ := os.Create("/tmp/kih.mp3")
			defer os.Remove("/tmp/kih.mp3")
			defer out.Close()

			resp, err := http.Get(url)
			check(err)
			defer resp.Body.Close()
	*/
	/*
		log.Printf("Processing %s", url)
		// io.Copy(out, resp.Body)
		metadata, err := audio.Identify("/tmp/kih.mp3")
		check(err)
		log.Printf("Meta %#v", metadata.String())
	*/
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
			fmt.Println(url)
		}
	}
}

func prune() {
	var (
		url string
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
			log.Printf("%s was removed from the server", url)
		}
	}
}

func usage() {
	println("Usage:")
	println("kih [fetch|sync|help]")
}

func makeRSS() {
	var (
		pubdate     string
		size        uint64
		title       string
		author      string
		subtitle    string
		summary     string
		image       string
		url         string
		typ         string
		guid        string
		description string
		duration    string
		explicit    string
		items       []podfeed.Item
		rows        *sql.Rows
	)

	rows, _ = db.Query(`SELECT
		pubdate, len, title, itunes_author, itunes_subtitle,
		itunes_summary, itunes_image, url, type, guid, description,
		itunes_duration, itunes_explicit
	  FROM episodes ORDER BY pubdate DESC`)
	defer rows.Close()

	for rows.Next() {
		if err := rows.Scan(&pubdate, &size, &title, &author, &subtitle,
			&summary, &image, &url, &typ, &guid,
			&description, &duration, &explicit); err != nil {
			log.Fatal(err)
		}

		items = append(items, podfeed.Item{
			Title:    title,
			PubDate:  podfeed.Time{Value: sqlDate(pubdate)},
			Link:     url,
			Author:   author,
			Image:    podfeed.Image{Href: image},
			Duration: duration,

			Enclosure: podfeed.Enclosure{
				Length: size,
				Url:    url,
				Type:   typ}})
	}

	t, _ := template.New("webpage").Parse(rssTpl)
	_ = t.Execute(os.Stdout, podfeed.Podcast{Items: items})
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
		makeRSS()
	case "id3":
		loadMetadata()
	case "prune":
		prune()
	default:
		usage()
	}
}
