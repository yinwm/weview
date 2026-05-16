package favorites

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"wxview/internal/sqlitecli"
)

type Item struct {
	ID            int64             `json:"id"`
	Type          string            `json:"type"`
	TypeCode      int64             `json:"type_code"`
	Time          string            `json:"time"`
	Timestamp     int64             `json:"timestamp"`
	Summary       string            `json:"summary"`
	Content       string            `json:"content,omitempty"`
	URL           string            `json:"url,omitempty"`
	ContentDetail map[string]string `json:"content_detail,omitempty"`
	ContentItems  []ContentItem     `json:"content_items,omitempty"`
	From          string            `json:"from_username,omitempty"`
	SourceChat    string            `json:"source_chat_username,omitempty"`
}

type ContentItem struct {
	Type               string `json:"type,omitempty"`
	Title              string `json:"title,omitempty"`
	Text               string `json:"text,omitempty"`
	Desc               string `json:"desc,omitempty"`
	DataType           string `json:"data_type,omitempty"`
	DataID             string `json:"data_id,omitempty"`
	DataSourceID       string `json:"data_source_id,omitempty"`
	HTMLID             string `json:"html_id,omitempty"`
	Format             string `json:"format,omitempty"`
	Size               string `json:"size,omitempty"`
	Duration           string `json:"duration,omitempty"`
	URL                string `json:"url,omitempty"`
	ThumbURL           string `json:"thumb_url,omitempty"`
	RemoteLocator      string `json:"remote_locator,omitempty"`
	ThumbRemoteLocator string `json:"thumb_remote_locator,omitempty"`
	RemoteLocatorKind  string `json:"remote_locator_kind,omitempty"`
	SourcePath         string `json:"source_path,omitempty"`
	ThumbSourcePath    string `json:"thumb_source_path,omitempty"`
	MD5                string `json:"md5,omitempty"`
	Head256MD5         string `json:"head256_md5,omitempty"`
	ThumbMD5           string `json:"thumb_md5,omitempty"`
	MessageUUID        string `json:"message_uuid,omitempty"`
	SourceName         string `json:"source_name,omitempty"`
	SourceCreateTime   int64  `json:"source_create_time,omitempty"`
	SourceTime         string `json:"source_time,omitempty"`
	MediaStatus        string `json:"media_status,omitempty"`
	MediaReason        string `json:"media_reason,omitempty"`
}

type QueryOptions struct {
	Type   string
	Query  string
	Limit  int
	Offset int
}

type Service struct {
	CacheDB string
	DataDir string
}

func NewService(cacheDB string) Service {
	return Service{CacheDB: cacheDB}
}

func NewServiceWithDataDir(cacheDB string, dataDir string) Service {
	return Service{CacheDB: cacheDB, DataDir: dataDir}
}

func (s Service) List(ctx context.Context, opts QueryOptions) ([]Item, error) {
	if s.CacheDB == "" {
		return nil, fmt.Errorf("favorite cache path is empty")
	}
	if _, err := os.Stat(s.CacheDB); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("favorite cache does not exist: run `wxview init` with process-memory permissions and retry")
		}
		return nil, err
	}
	if opts.Limit < 0 {
		return nil, fmt.Errorf("limit must be >= 0")
	}
	if opts.Offset < 0 {
		return nil, fmt.Errorf("offset must be >= 0")
	}
	typeCode, hasType, err := typeFilter(opts.Type)
	if err != nil {
		return nil, err
	}

	clauses := []string{}
	if hasType {
		clauses = append(clauses, fmt.Sprintf("type = %d", typeCode))
	}
	queryText := strings.TrimSpace(opts.Query)
	if queryText != "" {
		clauses = append(clauses, "content LIKE "+sqlQuote("%"+escapeLike(queryText)+"%")+" ESCAPE '\\'")
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
  COALESCE(type, 0) AS type,
  COALESCE(update_time, 0) AS update_time,
  COALESCE(content, '') AS content,
  COALESCE(fromusr, '') AS fromusr,
  COALESCE(realchatname, '') AS realchatname
FROM fav_db_item
%s
ORDER BY update_time DESC
%s;
`, whereSQL, limitSQL)
	cmd := exec.CommandContext(ctx, "sqlite3", "-json", sqlitecli.ImmutableURI(s.CacheDB), sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("query favorites: %v: %s", err, bytes.TrimSpace(out))
	}
	var rows []struct {
		ID         int64  `json:"local_id"`
		Type       int64  `json:"type"`
		UpdateTime int64  `json:"update_time"`
		Content    string `json:"content"`
		From       string `json:"fromusr"`
		SourceChat string `json:"realchatname"`
	}
	if len(bytes.TrimSpace(out)) > 0 {
		if err := json.Unmarshal(out, &rows); err != nil {
			return nil, fmt.Errorf("parse sqlite json: %w", err)
		}
	}
	items := make([]Item, 0, len(rows))
	for _, row := range rows {
		ts := normalizeTimestamp(row.UpdateTime)
		summary, content, url, detail, contentItems := s.parseFavoriteContent(row.Content, row.Type)
		items = append(items, Item{
			ID:            row.ID,
			Type:          typeLabel(row.Type),
			TypeCode:      row.Type,
			Timestamp:     ts,
			Time:          formatUnix(ts),
			Summary:       summary,
			Content:       content,
			URL:           url,
			ContentDetail: detail,
			ContentItems:  contentItems,
			From:          row.From,
			SourceChat:    row.SourceChat,
		})
	}
	return items, nil
}

func typeFilter(value string) (int64, bool, error) {
	switch strings.TrimSpace(value) {
	case "":
		return 0, false, nil
	case "text":
		return 1, true, nil
	case "image":
		return 2, true, nil
	case "voice":
		return 3, true, nil
	case "video":
		return 4, true, nil
	case "article":
		return 5, true, nil
	case "location":
		return 6, true, nil
	case "file":
		return 8, true, nil
	case "chat_history", "chat-history":
		return 14, true, nil
	case "note":
		return 18, true, nil
	case "card":
		return 19, true, nil
	case "video_channel", "video-channel", "channel", "finder":
		return 20, true, nil
	default:
		return 0, false, fmt.Errorf("invalid favorite type %q: use text, image, voice, video, article, location, file, chat_history, note, card, or video_channel", value)
	}
}

func typeLabel(code int64) string {
	switch code {
	case 1:
		return "text"
	case 2:
		return "image"
	case 3:
		return "voice"
	case 4:
		return "video"
	case 5:
		return "article"
	case 6:
		return "location"
	case 8:
		return "file"
	case 14:
		return "chat_history"
	case 18:
		return "note"
	case 19:
		return "card"
	case 20:
		return "video_channel"
	default:
		return "type_" + strconv.FormatInt(code, 10)
	}
}

type favItem struct {
	Title    string      `xml:"title"`
	Desc     string      `xml:"desc"`
	Page     favPage     `xml:"weburlitem"`
	Location favLocation `xml:"locitem"`
	Source   favSource   `xml:"source"`
	DataList favDataList `xml:"datalist"`
	DataItem favDataItem `xml:"dataitem"`
	Nickname string      `xml:"nickname"`
}

type favPage struct {
	Title   string `xml:"pagetitle"`
	Desc    string `xml:"pagedesc"`
	PageURL string `xml:"url"`
}

type favSource struct {
	SourceID     string `xml:"sourceid,attr"`
	SourceType   string `xml:"sourcetype,attr"`
	Link         string `xml:"link"`
	FromUser     string `xml:"fromusr"`
	ToUser       string `xml:"tousr"`
	RealChatName string `xml:"realchatname"`
	MsgID        string `xml:"msgid"`
	CreateTime   int64  `xml:"createtime"`
}

type favLocation struct {
	Label   string `xml:"label"`
	Lat     string `xml:"lat"`
	Lng     string `xml:"lng"`
	POIName string `xml:"poiname"`
}

type favDataList struct {
	Items []favDataItem `xml:"dataitem"`
}

type favDataItem struct {
	DataType             string  `xml:"datatype"`
	DataTypeAttr         string  `xml:"datatype,attr"`
	DataID               string  `xml:"dataid"`
	DataIDAttr           string  `xml:"dataid,attr"`
	DataSourceID         string  `xml:"datasourceid"`
	DataSourceIDAttr     string  `xml:"datasourceid,attr"`
	HTMLID               string  `xml:"htmlid"`
	HTMLIDAttr           string  `xml:"htmlid,attr"`
	DataFmt              string  `xml:"datafmt"`
	DataTitle            string  `xml:"datatitle"`
	DataDesc             string  `xml:"datadesc"`
	SourceName           string  `xml:"datasrcname"`
	StreamWebURL         string  `xml:"stream_weburl"`
	Link                 string  `xml:"link"`
	AppID                string  `xml:"appid"`
	FullSize             string  `xml:"fullsize"`
	Duration             string  `xml:"duration"`
	CDNDataURL           string  `xml:"cdn_dataurl"`
	CDNThumbURL          string  `xml:"cdn_thumburl"`
	SourceDataPath       string  `xml:"sourcedatapath"`
	SourceThumbPath      string  `xml:"sourcethumbpath"`
	MsgDataPath          string  `xml:"msgDataPath"`
	MsgThumbPath         string  `xml:"msgThumpPath"`
	FullMD5              string  `xml:"fullmd5"`
	Head256MD5           string  `xml:"head256md5"`
	ThumbFullMD5         string  `xml:"thumbfullmd5"`
	MessageUUID          string  `xml:"messageuuid"`
	SrcMsgCreateTime     string  `xml:"srcmsgcreatetime"`
	SrcMsgCreateTimeAttr string  `xml:"srcmsgcreatetime,attr"`
	DataSrcTime          string  `xml:"datasrctime"`
	DataSrcTimeAttr      string  `xml:"datasrctime,attr"`
	Page                 favPage `xml:"weburlitem"`
}

func (s Service) parseFavoriteContent(content string, typeCode int64) (string, string, string, map[string]string, []ContentItem) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", "", "", nil, nil
	}
	var item favItem
	if err := xml.Unmarshal([]byte(content), &item); err != nil {
		return trimSummary(content), strings.TrimSpace(content), "", nil, nil
	}
	if len(item.DataList.Items) == 0 && hasDataItem(item.DataItem) {
		item.DataList.Items = []favDataItem{item.DataItem}
	}
	summary, url := summarizeFavorite(item, typeCode)
	if summary == "" {
		summary = "[收藏]"
	}
	contentItems := contentItemsFromFavorite(typeCode, item)
	s.resolveLocalContentItems(typeCode, item, contentItems)
	readableContent := favoriteReadableContent(typeCode, item, contentItems)
	detail := favoriteDetail(typeCode, item, readableContent, contentItems)
	return summary, readableContent, url, detail, contentItems
}

func summarizeFavorite(item favItem, typeCode int64) (string, string) {
	switch typeCode {
	case 1:
		return firstNonEmpty(trimSummary(item.Desc), dataSummary(item, 1)), ""
	case 2:
		return labeledSummary("图片", dataSummary(item, 1)), ""
	case 3:
		return labeledSummary("语音", firstDataDuration(item)), ""
	case 4:
		return labeledSummary("视频", dataSummary(item, 1)), firstOpenURL(item)
	case 5:
		summary := strings.Join(compact(item.Page.Title, item.Page.Desc, item.Title, item.Desc, dataSummary(item, 1)), " - ")
		if summary == "" {
			summary = "[文章]"
		}
		return trimSummary(summary), firstNonEmpty(httpOnly(item.Source.Link), firstOpenURL(item))
	case 6:
		location := strings.Join(compact(item.Location.Label, item.Location.POIName, coordinateSummary(item.Location)), " - ")
		return labeledSummary("位置", location), ""
	case 8:
		return labeledSummary("文件", dataSummary(item, 1)), firstOpenURL(item)
	case 14:
		summary := firstNonEmpty(item.Title, item.Desc, dataSummary(item, 3))
		return labeledSummary("聊天记录", summary), firstOpenURL(item)
	case 18:
		summary := firstNonEmpty(item.Title, item.Desc, dataSummary(item, 3))
		return labeledSummary("笔记", summary), firstOpenURL(item)
	case 19:
		return firstNonEmpty(trimSummary(item.Desc), trimSummary(item.Title), labeledSummary("名片", dataSummary(item, 1))), ""
	case 20:
		summary := strings.Join(compact(item.Nickname, item.Title, item.Desc, dataSummary(item, 1)), " ")
		return labeledSummary("视频号", summary), firstOpenURL(item)
	default:
		return firstNonEmpty(trimSummary(item.Title), trimSummary(item.Desc), dataSummary(item, 2), "[收藏]"), firstOpenURL(item)
	}
}

func hasDataItem(item favDataItem) bool {
	return firstNonEmpty(dataType(item), dataID(item), dataSourceID(item), htmlID(item), item.DataFmt, item.DataTitle, item.DataDesc, item.SourceName, item.StreamWebURL, item.Link, item.AppID, item.FullSize, item.Duration, item.CDNDataURL, item.CDNThumbURL, item.SourceDataPath, item.SourceThumbPath, item.MsgDataPath, item.MsgThumbPath, item.Page.Title, item.Page.Desc) != ""
}

func dataSummary(item favItem, limit int) string {
	if limit <= 0 {
		limit = 1
	}
	values := make([]string, 0, limit)
	for _, data := range item.DataList.Items {
		title := firstNonEmpty(data.DataTitle, data.Page.Title)
		desc := firstNonEmpty(data.DataDesc, data.Page.Desc)
		value := strings.Join(compact(title, desc), " - ")
		if value == "" && data.DataFmt != "" {
			value = data.DataFmt
		}
		if value == "" {
			continue
		}
		values = append(values, trimSummary(value))
		if len(values) >= limit {
			break
		}
	}
	return strings.Join(values, " / ")
}

func firstOpenURL(item favItem) string {
	for _, data := range item.DataList.Items {
		if url := firstNonEmpty(httpOnly(data.StreamWebURL), httpOnly(data.Link), httpOnly(data.Page.PageURL)); url != "" {
			return url
		}
	}
	return ""
}

func firstRemoteLocator(item favItem) string {
	for _, data := range item.DataList.Items {
		if locator := firstNonEmpty(nonHTTP(data.CDNDataURL), nonHTTP(data.StreamWebURL), nonHTTP(data.Link), nonHTTP(data.Page.PageURL)); locator != "" {
			return locator
		}
	}
	return ""
}

func favoriteReadableContent(typeCode int64, item favItem, contentItems []ContentItem) string {
	switch typeCode {
	case 1:
		return firstNonEmpty(item.Desc, readableTextFromContentItems(contentItems))
	case 5:
		return strings.Join(compact(item.Page.Title, item.Page.Desc, item.Title, item.Desc, readableTextFromContentItems(contentItems)), "\n")
	case 6:
		return strings.Join(compact(item.Location.Label, item.Location.POIName, coordinateSummary(item.Location)), "\n")
	case 8:
		if len(contentItems) > 0 {
			first := contentItems[0]
			return firstNonEmpty(first.Title, first.Text, first.Desc)
		}
		return firstNonEmpty(item.Title, item.Desc)
	case 14, 18:
		return readableTextFromContentItems(contentItems)
	case 19:
		return firstNonEmpty(item.Desc, item.Title, readableTextFromContentItems(contentItems))
	case 20:
		return strings.Join(compact(item.Nickname, item.Title, item.Desc, readableTextFromContentItems(contentItems)), "\n")
	default:
		return firstNonEmpty(item.Desc, item.Title, readableTextFromContentItems(contentItems))
	}
}

func readableTextFromContentItems(contentItems []ContentItem) string {
	parts := make([]string, 0, len(contentItems))
	for _, item := range contentItems {
		text := firstNonEmpty(item.Text, item.Desc)
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, "\n\n")
}

func favoriteDetail(typeCode int64, item favItem, readableContent string, contentItems []ContentItem) map[string]string {
	detail := map[string]string{"type": typeLabel(typeCode)}
	putIfNotEmpty(detail, "title", item.Title)
	putIfNotEmpty(detail, "desc", trimSummary(item.Desc))
	putIfNotEmpty(detail, "text", readableContent)
	putIfNotEmpty(detail, "url", firstNonEmpty(httpOnly(item.Source.Link), firstOpenURL(item)))
	putIfNotEmpty(detail, "remote_locator", firstRemoteLocator(item))
	if len(contentItems) > 0 {
		detail["item_count"] = strconv.Itoa(len(contentItems))
		first := contentItems[0]
		putIfNotEmpty(detail, "format", first.Format)
		putIfNotEmpty(detail, "size", first.Size)
		putIfNotEmpty(detail, "duration", first.Duration)
		putIfNotEmpty(detail, "path", first.SourcePath)
		putIfNotEmpty(detail, "thumbnail_path", first.ThumbSourcePath)
		putIfNotEmpty(detail, "url", first.URL)
		putIfNotEmpty(detail, "thumb_url", first.ThumbURL)
		putIfNotEmpty(detail, "remote_locator", first.RemoteLocator)
		putIfNotEmpty(detail, "thumb_remote_locator", first.ThumbRemoteLocator)
		putIfNotEmpty(detail, "remote_locator_kind", first.RemoteLocatorKind)
		putIfNotEmpty(detail, "md5", first.MD5)
		putIfNotEmpty(detail, "message_uuid", first.MessageUUID)
		putIfNotEmpty(detail, "media_status", first.MediaStatus)
		putIfNotEmpty(detail, "media_reason", first.MediaReason)
	}
	if len(detail) == 1 {
		return nil
	}
	return detail
}

func contentItemsFromFavorite(typeCode int64, item favItem) []ContentItem {
	if len(item.DataList.Items) == 0 {
		return nil
	}
	out := make([]ContentItem, 0, len(item.DataList.Items))
	for _, data := range item.DataList.Items {
		openURL := firstNonEmpty(httpOnly(data.StreamWebURL), httpOnly(data.Link), httpOnly(data.Page.PageURL), httpOnly(data.CDNDataURL))
		thumbURL := httpOnly(data.CDNThumbURL)
		remoteLocator := firstNonEmpty(nonHTTP(data.CDNDataURL), nonHTTP(data.StreamWebURL), nonHTTP(data.Link), nonHTTP(data.Page.PageURL))
		thumbRemoteLocator := nonHTTP(data.CDNThumbURL)
		text := firstNonEmpty(data.DataDesc, data.Page.Desc)
		sourceCreateTime, _ := parsePositiveInt64(firstNonEmpty(data.SrcMsgCreateTime, data.SrcMsgCreateTimeAttr))
		contentItem := ContentItem{
			Type:               contentItemType(typeCode, data),
			Title:              firstNonEmpty(data.DataTitle, data.Page.Title),
			Text:               text,
			Desc:               text,
			DataType:           dataType(data),
			DataID:             dataID(data),
			DataSourceID:       dataSourceID(data),
			HTMLID:             htmlID(data),
			Format:             strings.TrimSpace(data.DataFmt),
			Size:               strings.TrimSpace(data.FullSize),
			Duration:           strings.TrimSpace(data.Duration),
			URL:                openURL,
			ThumbURL:           thumbURL,
			RemoteLocator:      remoteLocator,
			ThumbRemoteLocator: thumbRemoteLocator,
			SourcePath:         firstNonEmpty(data.SourceDataPath, data.MsgDataPath),
			ThumbSourcePath:    firstNonEmpty(data.SourceThumbPath, data.MsgThumbPath),
			MD5:                strings.TrimSpace(data.FullMD5),
			Head256MD5:         strings.TrimSpace(data.Head256MD5),
			ThumbMD5:           strings.TrimSpace(data.ThumbFullMD5),
			MessageUUID:        strings.TrimSpace(data.MessageUUID),
			SourceName:         strings.TrimSpace(data.SourceName),
			SourceCreateTime:   normalizeTimestamp(sourceCreateTime),
			SourceTime:         firstNonEmpty(data.DataSrcTime, data.DataSrcTimeAttr),
		}
		if contentItem.RemoteLocator != "" || contentItem.ThumbRemoteLocator != "" {
			contentItem.RemoteLocatorKind = "wechat_cdn_locator"
		}
		contentItem.MediaStatus, contentItem.MediaReason = mediaStatus(contentItem)
		out = append(out, contentItem)
	}
	return out
}

func (s Service) resolveLocalContentItems(typeCode int64, item favItem, contentItems []ContentItem) {
	if strings.TrimSpace(s.DataDir) == "" || len(contentItems) == 0 {
		return
	}
	for i := range contentItems {
		if contentItems[i].SourcePath != "" {
			contentItems[i].MediaStatus, contentItems[i].MediaReason = mediaStatus(contentItems[i])
			continue
		}
		if path, ok := s.resolveLocalContentItem(typeCode, item, contentItems[i]); ok {
			contentItems[i].SourcePath = path
			contentItems[i].MediaStatus, contentItems[i].MediaReason = mediaStatus(contentItems[i])
		}
	}
}

func (s Service) resolveLocalContentItem(typeCode int64, item favItem, contentItem ContentItem) (string, bool) {
	switch typeCode {
	case 8:
		return resolveLocalFavoriteFile(s.DataDir, item, contentItem)
	case 14, 18:
		if contentItem.Type == "file" {
			return resolveLocalFavoriteFile(s.DataDir, item, contentItem)
		}
		if contentItem.Type == "video" || contentItem.Type == "image" {
			return resolveLocalFavoriteMedia(s.DataDir, contentItem)
		}
	default:
		return "", false
	}
	return "", false
}

func resolveLocalFavoriteFile(dataDir string, item favItem, contentItem ContentItem) (string, bool) {
	accountBase := filepath.Dir(dataDir)
	if accountBase == "." || accountBase == dataDir {
		return "", false
	}
	title := firstNonEmpty(contentItem.Title, item.Title)
	if title == "" {
		return "", false
	}
	months := favoriteCandidateMonths(firstPositiveInt64(contentItem.SourceCreateTime, item.Source.CreateTime))
	for _, month := range months {
		path := filepath.Join(accountBase, "msg", "file", month, title)
		if favoriteFileMatches(path, contentItem.Size) {
			return path, true
		}
	}
	for _, month := range months {
		dir := filepath.Join(accountBase, "msg", "file", month)
		if path, ok := scanFavoriteFileDir(dir, title, contentItem.Size); ok {
			return path, true
		}
	}
	return "", false
}

func resolveLocalFavoriteMedia(dataDir string, contentItem ContentItem) (string, bool) {
	accountBase := filepath.Dir(dataDir)
	if accountBase == "." || accountBase == dataDir || contentItem.SourceCreateTime <= 0 {
		return "", false
	}
	if _, ok := parsePositiveInt64(contentItem.Size); !ok {
		return "", false
	}
	months := favoriteCandidateMonths(contentItem.SourceCreateTime)
	switch contentItem.Type {
	case "video":
		for _, month := range months {
			root := filepath.Join(accountBase, "msg", "video")
			if path, ok := scanFavoriteMediaTree(root, month, contentItem); ok {
				return path, true
			}
		}
	case "image":
		for _, month := range months {
			root := filepath.Join(accountBase, "msg", "attach")
			if path, ok := scanFavoriteImageTree(root, month, contentItem); ok {
				return path, true
			}
		}
	}
	return "", false
}

func favoriteCandidateMonths(createTime int64) []string {
	seen := map[string]bool{}
	add := func(t time.Time, out *[]string) {
		value := t.Format("2006-01")
		if !seen[value] {
			seen[value] = true
			*out = append(*out, value)
		}
	}
	out := make([]string, 0, 3)
	if createTime > 0 {
		t := time.Unix(normalizeTimestamp(createTime), 0)
		add(t, &out)
		add(t.AddDate(0, -1, 0), &out)
		add(t.AddDate(0, 1, 0), &out)
	}
	return out
}

func favoriteFileMatches(path string, sizeText string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	size, ok := parsePositiveInt64(sizeText)
	return !ok || size == info.Size()
}

func scanFavoriteFileDir(dir string, title string, sizeText string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	base := strings.TrimSuffix(title, filepath.Ext(title))
	ext := strings.ToLower(filepath.Ext(title))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.EqualFold(name, title) || (ext != "" && strings.HasPrefix(name, base+"(") && strings.EqualFold(filepath.Ext(name), ext)) {
			path := filepath.Join(dir, name)
			if favoriteFileMatches(path, sizeText) {
				return path, true
			}
		}
	}
	return "", false
}

func scanFavoriteMediaTree(root string, month string, contentItem ContentItem) (string, bool) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		if path, ok := scanFavoriteMediaDir(dir, contentItem); ok {
			return path, true
		}
		if path, ok := scanFavoriteMediaDir(filepath.Join(dir, month), contentItem); ok {
			return path, true
		}
	}
	return "", false
}

func scanFavoriteImageTree(root string, month string, contentItem ContentItem) (string, bool) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name(), month, "Img")
		if path, ok := scanFavoriteMediaDir(dir, contentItem); ok {
			return path, true
		}
	}
	return "", false
}

func scanFavoriteMediaDir(dir string, contentItem ContentItem) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if favoriteFileMatches(path, contentItem.Size) {
			return path, true
		}
	}
	return "", false
}

func parsePositiveInt64(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(value, 10, 64)
	return n, err == nil && n > 0
}

func firstPositiveInt64(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func contentItemType(typeCode int64, data favDataItem) string {
	switch typeCode {
	case 2:
		return "image"
	case 3:
		return "voice"
	case 4:
		return "video"
	case 5:
		return "article"
	case 8:
		return "file"
	case 14:
		return compoundContentItemType("chat_history_item", data)
	case 18:
		return compoundContentItemType("note_item", data)
	case 20:
		return "video_channel"
	}
	if strings.TrimSpace(data.DataFmt) != "" {
		return strings.TrimSpace(data.DataFmt)
	}
	return "item"
}

func compoundContentItemType(fallback string, data favDataItem) string {
	switch dataType(data) {
	case "1":
		return "text"
	case "2":
		return "image"
	case "3":
		return "voice"
	case "4":
		return "video"
	case "5":
		return "article"
	case "6":
		return "location"
	case "8":
		format := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(data.DataFmt), "."))
		if format == "htm" || format == "html" {
			return "html"
		}
		return "file"
	}
	if strings.TrimSpace(data.DataDesc) != "" && firstNonEmpty(data.DataFmt, data.CDNDataURL, data.CDNThumbURL, data.DataTitle) == "" {
		return "text"
	}
	return fallback
}

func dataType(data favDataItem) string {
	return firstNonEmpty(data.DataType, data.DataTypeAttr)
}

func dataID(data favDataItem) string {
	return firstNonEmpty(data.DataID, data.DataIDAttr)
}

func dataSourceID(data favDataItem) string {
	return firstNonEmpty(data.DataSourceID, data.DataSourceIDAttr)
}

func htmlID(data favDataItem) string {
	return firstNonEmpty(data.HTMLID, data.HTMLIDAttr)
}

func mediaStatus(item ContentItem) (string, string) {
	if item.SourcePath != "" || item.ThumbSourcePath != "" {
		return "local_path", "favorite XML includes a local source path"
	}
	if item.URL != "" || item.ThumbURL != "" {
		return "remote_only", "favorite XML includes remote URL metadata but no local source path"
	}
	if item.RemoteLocator != "" || item.ThumbRemoteLocator != "" {
		return "remote_only", "favorite XML includes WeChat CDN locator metadata but no local source path"
	}
	return "metadata_only", "favorite XML does not include a local source path or URL"
}

func httpOnly(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	return ""
}

func nonHTTP(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return ""
	}
	return value
}

func putIfNotEmpty(out map[string]string, key string, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		out[key] = value
	}
}

func firstDataDuration(item favItem) string {
	for _, data := range item.DataList.Items {
		if strings.TrimSpace(data.Duration) != "" {
			return strings.TrimSpace(data.Duration) + "s"
		}
	}
	return ""
}

func coordinateSummary(loc favLocation) string {
	lat := strings.TrimSpace(loc.Lat)
	lng := strings.TrimSpace(loc.Lng)
	if lat == "" || lng == "" {
		return ""
	}
	return lat + "," + lng
}

func labeledSummary(label string, detail string) string {
	detail = trimSummary(detail)
	if detail == "" {
		return "[" + label + "]"
	}
	return "[" + label + "] " + detail
}

func trimSummary(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	runes := []rune(value)
	if len(runes) > 160 {
		return string(runes[:160]) + "..."
	}
	return value
}

func normalizeTimestamp(ts int64) int64 {
	if ts > 9_999_999_999 {
		return ts / 1000
	}
	return ts
}

func formatUnix(ts int64) string {
	if ts <= 0 {
		return ""
	}
	return time.Unix(ts, 0).Format("2006-01-02 15:04:05")
}

func compact(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
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

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "%", "\\%")
	return strings.ReplaceAll(value, "_", "\\_")
}

func sqlQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
