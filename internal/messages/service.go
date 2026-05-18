package messages

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	zstdpkg "github.com/klauspost/compress/zstd"

	"wxview/internal/media"
	"wxview/internal/sqlitecli"
)

type Message struct {
	ID              string            `json:"id"`
	ChatUsername    string            `json:"chat_username"`
	ChatKind        string            `json:"chat_kind"`
	ChatDisplayName string            `json:"chat_display_name"`
	ChatAlias       string            `json:"chat_alias"`
	ChatRemark      string            `json:"chat_remark"`
	ChatNickName    string            `json:"chat_nick_name"`
	FromUsername    string            `json:"from_username"`
	Direction       string            `json:"direction"`
	IsSelf          bool              `json:"is_self"`
	IsChatroom      bool              `json:"is_chatroom"`
	Type            int64             `json:"type"`
	SubType         int64             `json:"sub_type"`
	Seq             int64             `json:"seq"`
	ServerID        int64             `json:"server_id"`
	CreateTime      int64             `json:"create_time"`
	Time            string            `json:"time"`
	Content         string            `json:"content"`
	ContentDetail   map[string]string `json:"content_detail,omitempty"`
	ContentEncoding string            `json:"content_encoding"`
	Source          *MessageSource    `json:"source,omitempty"`

	LocalID      int64  `json:"-"`
	RawType      int64  `json:"-"`
	Status       int64  `json:"-"`
	RealSenderID int64  `json:"-"`
	SourceDB     string `json:"-"`
	TableName    string `json:"-"`
	RawContent   string `json:"-"`
}

const ChatKindUnknown = "unknown"

type ChatInfo struct {
	Username    string
	Kind        string
	DisplayName string
	Alias       string
	Remark      string
	NickName    string
}

type MessageSource struct {
	DB           string `json:"db"`
	Table        string `json:"table"`
	LocalID      int64  `json:"local_id"`
	RawType      int64  `json:"raw_type"`
	Status       int64  `json:"status"`
	RealSenderID int64  `json:"real_sender_id"`
}

type QueryOptions struct {
	Username      string
	Start         int64
	End           int64
	AfterSeq      int64
	HasStart      bool
	HasEnd        bool
	HasAfterSeq   bool
	IncludeSource bool
	MediaResolver *media.Resolver
	Limit         int
	Offset        int
}

type SearchOptions struct {
	Chats         []ChatInfo
	Query         string
	Start         int64
	End           int64
	HasStart      bool
	HasEnd        bool
	IncludeSource bool
	Limit         int
	Offset        int
}

type RowRef struct {
	SourceDB     string `json:"source_db"`
	TableName    string `json:"table_name"`
	ChatUsername string `json:"chat_username"`
	LocalID      int64  `json:"local_id"`
}

type RefQueryOptions struct {
	IncludeSource bool
	MediaResolver *media.Resolver
}

type Service struct {
	CacheDBs []string
}

func NewService(cacheDBs []string) Service {
	return Service{CacheDBs: cacheDBs}
}

func (s Service) List(ctx context.Context, opts QueryOptions) ([]Message, error) {
	username := strings.TrimSpace(opts.Username)
	if username == "" {
		return nil, fmt.Errorf("username is required")
	}
	if opts.Limit < 0 {
		return nil, fmt.Errorf("limit must be >= 0")
	}
	if opts.Offset < 0 {
		return nil, fmt.Errorf("offset must be >= 0")
	}
	if opts.HasStart && opts.HasEnd && opts.Start > opts.End {
		return nil, fmt.Errorf("start must not be later than end")
	}
	if len(s.CacheDBs) == 0 {
		return nil, fmt.Errorf("message cache does not exist: run `wxview messages --refresh --username %s` first", username)
	}

	tableName := TableName(username)
	var out []Message
	for _, dbPath := range s.CacheDBs {
		if _, err := os.Stat(dbPath); err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("message cache missing: %s", dbPath)
			}
			return nil, err
		}
		exists, err := tableExists(ctx, dbPath, tableName)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		rows, err := queryDB(ctx, dbPath, tableName, opts)
		if err != nil {
			return nil, err
		}
		sourceDB := filepath.Base(dbPath)
		for _, row := range rows {
			out = append(out, normalizeRow(username, tableName, sourceDB, opts.IncludeSource, row))
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreateTime != out[j].CreateTime {
			return out[i].CreateTime < out[j].CreateTime
		}
		if out[i].Seq != out[j].Seq {
			return out[i].Seq < out[j].Seq
		}
		if out[i].LocalID != out[j].LocalID {
			return out[i].LocalID < out[j].LocalID
		}
		return out[i].SourceDB < out[j].SourceDB
	})
	page := paginate(out, opts.Limit, opts.Offset)
	if opts.MediaResolver != nil {
		for i := range page {
			enrichMediaDetail(&page[i], opts.MediaResolver)
		}
	}
	return page, nil
}

func (s Service) ListByRefs(ctx context.Context, refs []RowRef, opts RefQueryOptions) ([]Message, error) {
	if len(refs) == 0 {
		return []Message{}, nil
	}
	dbBySource := make(map[string]string, len(s.CacheDBs))
	for _, dbPath := range s.CacheDBs {
		dbBySource[filepath.Base(dbPath)] = dbPath
	}
	type groupKey struct {
		sourceDB  string
		tableName string
	}
	idsByGroup := make(map[groupKey][]int64)
	for _, ref := range refs {
		dbPath := dbBySource[ref.SourceDB]
		if dbPath == "" {
			return nil, fmt.Errorf("indexed source DB is not in current message caches: %s", ref.SourceDB)
		}
		key := groupKey{sourceDB: ref.SourceDB, tableName: ref.TableName}
		idsByGroup[key] = append(idsByGroup[key], ref.LocalID)
	}

	rowsByRef := make(map[string]queryRow, len(refs))
	for key, ids := range idsByGroup {
		dbPath := dbBySource[key.sourceDB]
		rows, err := queryRowsByLocalIDs(ctx, dbPath, key.tableName, ids)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			rowsByRef[refKey(key.sourceDB, key.tableName, row.LocalID)] = row
		}
	}

	out := make([]Message, 0, len(refs))
	for _, ref := range refs {
		row, ok := rowsByRef[refKey(ref.SourceDB, ref.TableName, ref.LocalID)]
		if !ok {
			return nil, fmt.Errorf("indexed message row missing: %s %s local_id=%d", ref.SourceDB, ref.TableName, ref.LocalID)
		}
		out = append(out, normalizeRow(ref.ChatUsername, ref.TableName, ref.SourceDB, opts.IncludeSource, row))
	}
	if opts.MediaResolver != nil {
		for i := range out {
			enrichMediaDetail(&out[i], opts.MediaResolver)
		}
	}
	return out, nil
}

func (s Service) Search(ctx context.Context, opts SearchOptions) ([]Message, int, error) {
	needle := strings.ToLower(strings.TrimSpace(opts.Query))
	if needle == "" {
		return nil, 0, fmt.Errorf("query is required")
	}
	if opts.Limit < 0 {
		return nil, 0, fmt.Errorf("limit must be >= 0")
	}
	if opts.Offset < 0 {
		return nil, 0, fmt.Errorf("offset must be >= 0")
	}
	if opts.HasStart && opts.HasEnd && opts.Start > opts.End {
		return nil, 0, fmt.Errorf("start must not be later than end")
	}
	if len(opts.Chats) == 0 {
		return []Message{}, 0, nil
	}

	chatInfo := make(map[string]ChatInfo, len(opts.Chats))
	var matched []Message
	for _, chat := range opts.Chats {
		username := strings.TrimSpace(chat.Username)
		if username == "" {
			continue
		}
		chatInfo[username] = chat
		rows, err := s.List(ctx, QueryOptions{
			Username:      username,
			Start:         opts.Start,
			End:           opts.End,
			HasStart:      opts.HasStart,
			HasEnd:        opts.HasEnd,
			IncludeSource: opts.IncludeSource,
		})
		if err != nil {
			return nil, 0, err
		}
		for _, row := range rows {
			if MessageMatches(row, needle) {
				matched = append(matched, row)
			}
		}
	}
	ApplyChatInfo(matched, chatInfo)
	sort.SliceStable(matched, func(i, j int) bool {
		if matched[i].CreateTime != matched[j].CreateTime {
			return matched[i].CreateTime > matched[j].CreateTime
		}
		if matched[i].Seq != matched[j].Seq {
			return matched[i].Seq > matched[j].Seq
		}
		if matched[i].LocalID != matched[j].LocalID {
			return matched[i].LocalID > matched[j].LocalID
		}
		return matched[i].SourceDB < matched[j].SourceDB
	})
	total := len(matched)
	return paginate(matched, opts.Limit, opts.Offset), total, nil
}

func TableName(username string) string {
	sum := md5.Sum([]byte(username))
	return "Msg_" + hex.EncodeToString(sum[:])
}

func MessageMatches(message Message, needle string) bool {
	needle = strings.ToLower(strings.TrimSpace(needle))
	if needle == "" {
		return false
	}
	if strings.Contains(strings.ToLower(message.Content), needle) {
		return true
	}
	for _, value := range message.ContentDetail {
		if strings.Contains(strings.ToLower(value), needle) {
			return true
		}
	}
	return false
}

func SearchText(chatUsername string, localType int64, status int64, realSenderID int64, senderName string, decodedContent string) string {
	decodedContent = strings.ToValidUTF8(decodedContent, "")
	_, _, _, bodyContent := inferSender(chatUsername, status, realSenderID, senderName, decodedContent)
	msgType, subType := splitLocalType(localType)
	detail := normalizeContent(msgType, subType, bodyContent).Detail()
	parts := []string{}
	if !looksLikeXML(bodyContent) {
		parts = append(parts, decodedContent)
	}
	for _, value := range detail {
		if strings.TrimSpace(value) != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, "\n")
}

func looksLikeXML(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "<") || strings.HasPrefix(value, "<?xml")
}

func tableExists(ctx context.Context, dbPath string, tableName string) (bool, error) {
	query := fmt.Sprintf("SELECT name FROM sqlite_master WHERE type='table' AND name='%s' LIMIT 1;", tableName)
	cmd := exec.CommandContext(ctx, "sqlite3", "-json", sqlitecli.ImmutableURI(dbPath), query)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("query message table in %s: %v: %s", dbPath, err, bytes.TrimSpace(out))
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return false, nil
	}
	var rows []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(out, &rows); err != nil {
		return false, fmt.Errorf("parse sqlite json: %w", err)
	}
	return len(rows) > 0, nil
}

type queryRow struct {
	LocalID      int64  `json:"local_id"`
	LocalType    int64  `json:"local_type"`
	SortSeq      int64  `json:"sort_seq"`
	ServerID     int64  `json:"server_id"`
	RealSenderID int64  `json:"real_sender_id"`
	SenderName   string `json:"sender_name"`
	Status       int64  `json:"status"`
	CreateTime   int64  `json:"create_time"`
	ContentHex   string `json:"content_hex"`
}

func queryDB(ctx context.Context, dbPath string, tableName string, opts QueryOptions) ([]queryRow, error) {
	whereSQL := ""
	var clauses []string
	if opts.HasStart {
		clauses = append(clauses, fmt.Sprintf("create_time >= %d", opts.Start))
	}
	if opts.HasEnd {
		clauses = append(clauses, fmt.Sprintf("create_time <= %d", opts.End))
	}
	if opts.HasAfterSeq {
		clauses = append(clauses, fmt.Sprintf("sort_seq > %d", opts.AfterSeq))
	}
	if len(clauses) > 0 {
		whereSQL = "WHERE " + strings.Join(clauses, " AND ")
	}

	query := fmt.Sprintf(`
SELECT
  local_id,
  COALESCE(local_type, 0) AS local_type,
  COALESCE(sort_seq, 0) AS sort_seq,
  COALESCE(server_id, 0) AS server_id,
  COALESCE(real_sender_id, 0) AS real_sender_id,
  COALESCE((SELECT user_name FROM Name2Id WHERE rowid = COALESCE(real_sender_id, 0)), '') AS sender_name,
  COALESCE(status, 0) AS status,
  COALESCE(create_time, 0) AS create_time,
  hex(COALESCE(message_content, X'')) AS content_hex
FROM [%s]
%s
ORDER BY create_time ASC, sort_seq ASC, local_id ASC;
`, tableName, whereSQL)

	cmd := exec.CommandContext(ctx, "sqlite3", "-json", sqlitecli.ImmutableURI(dbPath), query)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("query messages in %s: %v: %s", dbPath, err, bytes.TrimSpace(out))
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return nil, nil
	}
	var rows []queryRow
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, fmt.Errorf("parse sqlite json: %w", err)
	}
	return rows, nil
}

func queryRowsByLocalIDs(ctx context.Context, dbPath string, tableName string, localIDs []int64) ([]queryRow, error) {
	if len(localIDs) == 0 {
		return nil, nil
	}
	ids := append([]int64{}, localIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	query := fmt.Sprintf(`
SELECT
  local_id,
  COALESCE(local_type, 0) AS local_type,
  COALESCE(sort_seq, 0) AS sort_seq,
  COALESCE(server_id, 0) AS server_id,
  COALESCE(real_sender_id, 0) AS real_sender_id,
  COALESCE((SELECT user_name FROM Name2Id WHERE rowid = COALESCE(real_sender_id, 0)), '') AS sender_name,
  COALESCE(status, 0) AS status,
  COALESCE(create_time, 0) AS create_time,
  hex(COALESCE(message_content, X'')) AS content_hex
FROM [%s]
WHERE local_id IN (%s);
`, tableName, strings.Join(parts, ","))

	cmd := exec.CommandContext(ctx, "sqlite3", "-json", sqlitecli.ImmutableURI(dbPath), query)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("query indexed messages in %s: %v: %s", dbPath, err, bytes.TrimSpace(out))
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return nil, nil
	}
	var rows []queryRow
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, fmt.Errorf("parse sqlite json: %w", err)
	}
	return rows, nil
}

func refKey(sourceDB string, tableName string, localID int64) string {
	return sourceDB + "\x00" + tableName + "\x00" + strconv.FormatInt(localID, 10)
}

func normalizeRow(username string, tableName string, sourceDB string, includeSource bool, row queryRow) Message {
	content, encoding := decodeContentHex(row.ContentHex)
	content = strings.ToValidUTF8(content, "")
	rawContent := content
	fromUsername, direction, isSelf, bodyContent := inferSender(username, row.Status, row.RealSenderID, row.SenderName, content)
	msgType, subType := splitLocalType(row.LocalType)
	normalized := normalizeContent(msgType, subType, bodyContent)
	msg := Message{
		ID:              stableID(sourceDB, tableName, row.LocalID),
		ChatUsername:    username,
		ChatKind:        ChatKindUnknown,
		ChatDisplayName: username,
		FromUsername:    fromUsername,
		Direction:       direction,
		IsSelf:          isSelf,
		IsChatroom:      strings.HasSuffix(username, "@chatroom"),
		Type:            msgType,
		SubType:         subType,
		Seq:             row.SortSeq,
		ServerID:        row.ServerID,
		CreateTime:      row.CreateTime,
		Time:            formatUnix(row.CreateTime),
		Content:         rawContent,
		ContentDetail:   normalized.Detail(),
		ContentEncoding: encoding,
		LocalID:         row.LocalID,
		RawType:         row.LocalType,
		Status:          row.Status,
		RealSenderID:    row.RealSenderID,
		SourceDB:        sourceDB,
		TableName:       tableName,
		RawContent:      rawContent,
	}
	if includeSource {
		msg.Source = &MessageSource{
			DB:           sourceDB,
			Table:        tableName,
			LocalID:      row.LocalID,
			RawType:      row.LocalType,
			Status:       row.Status,
			RealSenderID: row.RealSenderID,
		}
	}
	return msg
}

func ApplyChatInfo(list []Message, chatInfo map[string]ChatInfo) {
	for i := range list {
		ApplyChatInfoToMessage(&list[i], chatInfo[list[i].ChatUsername])
	}
}

func ApplyChatInfoToMessage(msg *Message, info ChatInfo) {
	if msg == nil {
		return
	}
	if strings.TrimSpace(info.Username) == "" {
		if strings.TrimSpace(msg.ChatKind) == "" {
			msg.ChatKind = ChatKindUnknown
		}
		if strings.TrimSpace(msg.ChatDisplayName) == "" {
			msg.ChatDisplayName = msg.ChatUsername
		}
		return
	}
	msg.ChatKind = defaultString(info.Kind, ChatKindUnknown)
	msg.ChatDisplayName = defaultString(info.DisplayName, msg.ChatUsername)
	msg.ChatAlias = info.Alias
	msg.ChatRemark = info.Remark
	msg.ChatNickName = info.NickName
}

func EnrichMediaDetails(list []Message, mediaResolver *media.Resolver) {
	if mediaResolver == nil {
		return
	}
	for i := range list {
		enrichMediaDetail(&list[i], mediaResolver)
	}
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func enrichMediaDetail(msg *Message, mediaResolver *media.Resolver) {
	if msg == nil || mediaResolver == nil {
		return
	}
	kind := mediaKindForMessageType(msg.Type, msg.SubType)
	if kind == "" {
		return
	}
	mediaInfo := mediaResolver.Resolve(kind, msg.ChatUsername, msg.LocalID, msg.ServerID, msg.CreateTime, msg.RawType, msg.RawContent, msg.IsChatroom)
	if mediaInfo.Status == "" {
		return
	}
	if msg.ContentDetail == nil {
		msg.ContentDetail = map[string]string{}
	}
	msg.ContentDetail["media_status"] = mediaInfo.Status
	msg.ContentDetail["decoded"] = fmt.Sprintf("%t", mediaInfo.Decoded)
	msg.ContentDetail["thumbnail"] = fmt.Sprintf("%t", mediaInfo.Thumbnail)
	if mediaInfo.Path != "" {
		msg.ContentDetail["path"] = mediaInfo.Path
	}
	if mediaInfo.SourcePath != "" {
		msg.ContentDetail["source_path"] = mediaInfo.SourcePath
	}
	if mediaInfo.ThumbnailPath != "" {
		msg.ContentDetail["thumbnail_path"] = mediaInfo.ThumbnailPath
	}
	if mediaInfo.ThumbnailSourcePath != "" {
		msg.ContentDetail["thumbnail_source_path"] = mediaInfo.ThumbnailSourcePath
	}
	if mediaInfo.ThumbnailDecoded {
		msg.ContentDetail["thumbnail_decoded"] = fmt.Sprintf("%t", mediaInfo.ThumbnailDecoded)
	}
	if mediaInfo.Width > 0 {
		msg.ContentDetail["width"] = fmt.Sprintf("%d", mediaInfo.Width)
	}
	if mediaInfo.Height > 0 {
		msg.ContentDetail["height"] = fmt.Sprintf("%d", mediaInfo.Height)
	}
	if mediaInfo.Reason != "" {
		msg.ContentDetail["media_reason"] = mediaInfo.Reason
	}
}

func mediaKindForMessageType(msgType int64, subType int64) string {
	switch msgType {
	case messageTypeImage:
		return "image"
	case messageTypeVoice:
		return "voice"
	case messageTypeVideo:
		return "video"
	case messageTypeShare:
		if subType == messageSubTypeFile {
			return "file"
		}
		return ""
	default:
		return ""
	}
}

func inferSender(username string, status int64, realSenderID int64, realSenderName string, content string) (string, string, bool, string) {
	isChatroom := strings.HasSuffix(username, "@chatroom")
	isSelf := status == 2 || (!isChatroom && realSenderName != "" && username != realSenderName) || (realSenderID == 2 && realSenderName == "")
	senderName := realSenderName
	if senderName == "" && !isSelf {
		senderName = username
	}
	if strings.HasSuffix(username, "@chatroom") {
		if sender, text, ok := strings.Cut(content, ":\n"); ok && sender != "" {
			return sender, directionFor(isSelf, true), isSelf, text
		}
		if isSelf {
			return selfSenderName(senderName), "out", true, content
		}
		if realSenderName != "" {
			return realSenderName, "in", false, content
		}
		return "", "unknown", false, content
	}
	if isSelf {
		return selfSenderName(senderName), "out", true, content
	}
	return senderName, "in", false, content
}

func selfSenderName(senderName string) string {
	if strings.TrimSpace(senderName) != "" {
		return senderName
	}
	return "self"
}

func directionFor(isSelf bool, hasSender bool) string {
	if isSelf {
		return "out"
	}
	if hasSender {
		return "in"
	}
	return "unknown"
}

func stableID(sourceDB string, tableName string, localID int64) string {
	return fmt.Sprintf("wechat:msg:%s:%s:%d", sourceDB, tableName, localID)
}

func splitLocalType(raw int64) (int64, int64) {
	value := uint64(raw)
	return int64(value & 0xffffffff), int64(value >> 32)
}

var (
	zstdMagic      = []byte{0x28, 0xb5, 0x2f, 0xfd}
	zstdDecoder    *zstdpkg.Decoder
	zstdDecoderErr error
)

func init() {
	zstdDecoder, zstdDecoderErr = zstdpkg.NewReader(nil)
}

func decodeContentHex(contentHex string) (string, string) {
	if contentHex == "" {
		return "", "text"
	}
	raw, err := hex.DecodeString(contentHex)
	if err != nil {
		return "", "invalid_hex"
	}
	return DecodeContent(raw)
}

func DecodeContent(raw []byte) (string, string) {
	if len(raw) == 0 {
		return "", "text"
	}
	if !bytes.HasPrefix(raw, zstdMagic) {
		return string(raw), "text"
	}
	if zstdDecoderErr != nil {
		return "", "zstd_error"
	}
	out, err := zstdDecoder.DecodeAll(raw, nil)
	if err != nil {
		return "", "zstd_error"
	}
	return string(out), "zstd"
}

func formatUnix(ts int64) string {
	if ts <= 0 {
		return ""
	}
	return time.Unix(ts, 0).Format("2006-01-02 15:04:05")
}

func paginate(list []Message, limit int, offset int) []Message {
	if offset < 0 {
		offset = 0
	}
	if limit < 0 {
		limit = 0
	}
	if offset >= len(list) {
		return []Message{}
	}
	if limit == 0 {
		return list[offset:]
	}
	end := offset + limit
	if end > len(list) {
		end = len(list)
	}
	return list[offset:end]
}
