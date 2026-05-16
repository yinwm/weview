package sns

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"wxview/internal/sqlitecli"
)

type Post struct {
	ID             int64   `json:"id"`
	AuthorUsername string  `json:"author_username"`
	Content        string  `json:"content"`
	Location       string  `json:"location,omitempty"`
	Timestamp      int64   `json:"timestamp"`
	Time           string  `json:"time"`
	MediaCount     int     `json:"media_count"`
	Media          []Media `json:"media,omitempty"`
}

type Media struct {
	Type          string `json:"type,omitempty"`
	SubType       string `json:"sub_type,omitempty"`
	URL           string `json:"url,omitempty"`
	Thumb         string `json:"thumb,omitempty"`
	MD5           string `json:"md5,omitempty"`
	Width         string `json:"width,omitempty"`
	Height        string `json:"height,omitempty"`
	TotalSize     string `json:"total_size,omitempty"`
	VideoMD5      string `json:"video_md5,omitempty"`
	VideoDuration string `json:"video_duration,omitempty"`
}

type Notification struct {
	ID                 int64  `json:"id"`
	Type               string `json:"type"`
	FromUsername       string `json:"from_username"`
	FromNickName       string `json:"from_nick_name"`
	Content            string `json:"content,omitempty"`
	FeedID             int64  `json:"feed_id"`
	FeedAuthorUsername string `json:"feed_author_username,omitempty"`
	FeedPreview        string `json:"feed_preview,omitempty"`
	Timestamp          int64  `json:"timestamp"`
	Time               string `json:"time"`
}

type QueryOptions struct {
	Username    string
	Query       string
	Start       int64
	End         int64
	HasStart    bool
	HasEnd      bool
	Limit       int
	Offset      int
	IncludeRead bool
}

type Service struct {
	CacheDB string
}

func NewService(cacheDB string) Service {
	return Service{CacheDB: cacheDB}
}

func (s Service) Feed(ctx context.Context, opts QueryOptions) ([]Post, error) {
	rows, err := s.queryTimeline(ctx, opts)
	if err != nil {
		return nil, err
	}
	posts := make([]Post, 0, len(rows))
	needle := strings.ToLower(strings.TrimSpace(opts.Query))
	username := strings.TrimSpace(opts.Username)
	for _, row := range rows {
		post := parsePost(row.ID, row.AuthorUsername, row.Content)
		if username != "" && post.AuthorUsername != username {
			continue
		}
		if opts.HasStart && post.Timestamp < opts.Start {
			continue
		}
		if opts.HasEnd && post.Timestamp > opts.End {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(post.Content), needle) {
			continue
		}
		posts = append(posts, post)
	}
	sort.SliceStable(posts, func(i, j int) bool {
		if posts[i].Timestamp != posts[j].Timestamp {
			return posts[i].Timestamp > posts[j].Timestamp
		}
		return posts[i].ID > posts[j].ID
	})
	return paginatePosts(posts, opts.Limit, opts.Offset), nil
}

func (s Service) Notifications(ctx context.Context, opts QueryOptions) ([]Notification, error) {
	if err := s.ensureCache(); err != nil {
		return nil, err
	}
	if opts.Limit < 0 || opts.Offset < 0 {
		return nil, fmt.Errorf("limit and offset must be >= 0")
	}
	clauses := []string{}
	if !opts.IncludeRead {
		clauses = append(clauses, "COALESCE(is_unread, 0) = 1")
	}
	if opts.HasStart {
		clauses = append(clauses, fmt.Sprintf("create_time >= %d", opts.Start))
	}
	if opts.HasEnd {
		clauses = append(clauses, fmt.Sprintf("create_time <= %d", opts.End))
	}
	whereSQL := ""
	if len(clauses) > 0 {
		whereSQL = "WHERE " + strings.Join(clauses, " AND ")
	}
	limitSQL := ""
	if opts.Limit > 0 {
		limitSQL = fmt.Sprintf("LIMIT %d OFFSET %d", opts.Limit, opts.Offset)
	} else if opts.Offset > 0 {
		limitSQL = fmt.Sprintf("LIMIT -1 OFFSET %d", opts.Offset)
	}
	sql := fmt.Sprintf(`
SELECT
  COALESCE(local_id, 0) AS local_id,
  COALESCE(create_time, 0) AS create_time,
  COALESCE(feed_id, 0) AS feed_id,
  COALESCE(from_username, '') AS from_username,
  COALESCE(from_nickname, '') AS from_nickname,
  COALESCE(content, '') AS content
FROM SnsMessage_tmp3
%s
ORDER BY create_time DESC
%s;
`, whereSQL, limitSQL)
	cmd := exec.CommandContext(ctx, "sqlite3", "-json", sqlitecli.ImmutableURI(s.CacheDB), sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("query sns notifications: %v: %s", err, bytes.TrimSpace(out))
	}
	var rows []notificationRow
	if len(bytes.TrimSpace(out)) > 0 {
		if err := json.Unmarshal(out, &rows); err != nil {
			return nil, fmt.Errorf("parse sqlite json: %w", err)
		}
	}
	feeds := s.feedPreviewMap(ctx, feedIDs(rows))
	items := make([]Notification, 0, len(rows))
	for _, row := range rows {
		kind := "comment"
		if strings.TrimSpace(row.Content) == "" {
			kind = "like"
		}
		preview := feeds[row.FeedID]
		items = append(items, Notification{
			ID:                 row.ID,
			Type:               kind,
			FromUsername:       row.FromUsername,
			FromNickName:       row.FromNickName,
			Content:            row.Content,
			FeedID:             row.FeedID,
			FeedAuthorUsername: preview.AuthorUsername,
			FeedPreview:        preview.Content,
			Timestamp:          row.CreateTime,
			Time:               formatUnix(row.CreateTime),
		})
	}
	return items, nil
}

type timelineRow struct {
	ID             int64  `json:"tid"`
	AuthorUsername string `json:"user_name"`
	Content        string `json:"content"`
}

type notificationRow struct {
	ID           int64  `json:"local_id"`
	CreateTime   int64  `json:"create_time"`
	FeedID       int64  `json:"feed_id"`
	FromUsername string `json:"from_username"`
	FromNickName string `json:"from_nickname"`
	Content      string `json:"content"`
}

func (s Service) queryTimeline(ctx context.Context, opts QueryOptions) ([]timelineRow, error) {
	if err := s.ensureCache(); err != nil {
		return nil, err
	}
	if opts.Limit < 0 || opts.Offset < 0 {
		return nil, fmt.Errorf("limit and offset must be >= 0")
	}
	sql := `
SELECT
  COALESCE(tid, 0) AS tid,
  COALESCE(user_name, '') AS user_name,
  COALESCE(content, '') AS content
FROM SnsTimeLine
ORDER BY tid DESC;
`
	cmd := exec.CommandContext(ctx, "sqlite3", "-json", sqlitecli.ImmutableURI(s.CacheDB), sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("query sns timeline: %v: %s", err, bytes.TrimSpace(out))
	}
	var rows []timelineRow
	if len(bytes.TrimSpace(out)) > 0 {
		if err := json.Unmarshal(out, &rows); err != nil {
			return nil, fmt.Errorf("parse sqlite json: %w", err)
		}
	}
	return rows, nil
}

func (s Service) ensureCache() error {
	if s.CacheDB == "" {
		return fmt.Errorf("sns cache path is empty")
	}
	if _, err := os.Stat(s.CacheDB); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("sns cache does not exist: run `wxview init` with process-memory permissions and retry")
		}
		return err
	}
	return nil
}

type timelineObject struct {
	Username      string        `xml:"username"`
	CreateTime    int64         `xml:"createTime"`
	ContentDesc   string        `xml:"contentDesc"`
	Location      locationNode  `xml:"location"`
	ContentObject contentObject `xml:"ContentObject"`
}

type locationNode struct {
	POIName string `xml:"poiName,attr"`
}

type contentObject struct {
	MediaList mediaList `xml:"mediaList"`
}

type mediaList struct {
	Media []mediaNode `xml:"media"`
}

type mediaNode struct {
	Type     string   `xml:"type"`
	SubType  string   `xml:"sub_type"`
	URL      attrText `xml:"url"`
	Thumb    attrText `xml:"thumb"`
	Size     sizeNode `xml:"size"`
	VideoMD5 string   `xml:"videomd5"`
	Duration string   `xml:"videoDuration"`
}

type attrText struct {
	Text string `xml:",chardata"`
	MD5  string `xml:"md5,attr"`
}

type sizeNode struct {
	Width     string `xml:"width,attr"`
	Height    string `xml:"height,attr"`
	TotalSize string `xml:"totalSize,attr"`
}

func parsePost(id int64, authorUsername string, raw string) Post {
	obj := parseTimelineObject(raw)
	author := firstNonEmpty(authorUsername, obj.Username)
	media := make([]Media, 0, len(obj.ContentObject.MediaList.Media))
	for _, item := range obj.ContentObject.MediaList.Media {
		media = append(media, Media{
			Type:          strings.TrimSpace(item.Type),
			SubType:       strings.TrimSpace(item.SubType),
			URL:           strings.TrimSpace(item.URL.Text),
			Thumb:         strings.TrimSpace(item.Thumb.Text),
			MD5:           strings.TrimSpace(item.URL.MD5),
			Width:         strings.TrimSpace(item.Size.Width),
			Height:        strings.TrimSpace(item.Size.Height),
			TotalSize:     strings.TrimSpace(item.Size.TotalSize),
			VideoMD5:      strings.TrimSpace(item.VideoMD5),
			VideoDuration: strings.TrimSpace(item.Duration),
		})
	}
	return Post{
		ID:             id,
		AuthorUsername: strings.TrimSpace(author),
		Content:        strings.TrimSpace(obj.ContentDesc),
		Location:       strings.TrimSpace(obj.Location.POIName),
		Timestamp:      obj.CreateTime,
		Time:           formatUnix(obj.CreateTime),
		MediaCount:     len(media),
		Media:          media,
	}
}

func parseTimelineObject(raw string) timelineObject {
	var obj timelineObject
	if err := xml.Unmarshal([]byte(raw), &obj); err == nil && (obj.CreateTime != 0 || obj.ContentDesc != "" || obj.Username != "") {
		return obj
	}
	var wrapper struct {
		Timeline timelineObject `xml:"TimelineObject"`
	}
	if err := xml.Unmarshal([]byte(raw), &wrapper); err == nil {
		return wrapper.Timeline
	}
	return timelineObject{}
}

type preview struct {
	AuthorUsername string
	Content        string
}

func (s Service) feedPreviewMap(ctx context.Context, ids []int64) map[int64]preview {
	if len(ids) == 0 {
		return nil
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf("%d", id))
	}
	sql := fmt.Sprintf(`
SELECT
  COALESCE(tid, 0) AS tid,
  COALESCE(user_name, '') AS user_name,
  COALESCE(content, '') AS content
FROM SnsTimeLine
WHERE tid IN (%s);
`, strings.Join(parts, ","))
	cmd := exec.CommandContext(ctx, "sqlite3", "-json", sqlitecli.ImmutableURI(s.CacheDB), sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil
	}
	var rows []timelineRow
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil
	}
	outMap := make(map[int64]preview, len(rows))
	for _, row := range rows {
		post := parsePost(row.ID, row.AuthorUsername, row.Content)
		text := post.Content
		if len([]rune(text)) > 60 {
			runes := []rune(text)
			text = string(runes[:60])
		}
		outMap[row.ID] = preview{AuthorUsername: post.AuthorUsername, Content: text}
	}
	return outMap
}

func feedIDs(rows []notificationRow) []int64 {
	set := map[int64]bool{}
	for _, row := range rows {
		if row.FeedID > 0 {
			set[row.FeedID] = true
		}
	}
	ids := make([]int64, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func paginatePosts(posts []Post, limit int, offset int) []Post {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(posts) {
		return []Post{}
	}
	if limit <= 0 {
		return posts[offset:]
	}
	end := offset + limit
	if end > len(posts) {
		end = len(posts)
	}
	return posts[offset:end]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func formatUnix(ts int64) string {
	if ts <= 0 {
		return ""
	}
	return time.Unix(ts, 0).Format("2006-01-02 15:04:05")
}
