package media

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"wxview/internal/sqlitedb"
)

func (r Resolver) ResolveVoice(chatUsername string, localID int64, serverID int64, rawType int64) Info {
	if int64(uint64(rawType)&0xffffffff) != 34 {
		return notFound("voice", "not a voice message")
	}
	for _, dbPath := range r.ResourceDBs {
		if _, err := os.Stat(dbPath); err != nil {
			continue
		}
		if serverID > 0 {
			if data, ok := queryVoiceByServerID(dbPath, serverID); ok {
				return r.writeVoiceData(dbPath, data, fmt.Sprintf("svr:%d", serverID))
			}
		}
		if chatUsername != "" && localID > 0 {
			if chatID, ok := queryChatNameID(dbPath, chatUsername); ok {
				if data, ok := queryVoiceByLocalID(dbPath, chatID, localID); ok {
					return r.writeVoiceData(dbPath, data, fmt.Sprintf("chat:%d:%d", chatID, localID))
				}
			}
		}
	}
	return notFound("voice", "voice_data not found in media_*.db")
}

func (r Resolver) writeVoiceData(sourcePath string, data []byte, cacheKey string) Info {
	if len(data) == 0 {
		return notFound("voice", "empty voice_data")
	}
	out := data
	if bytes.HasPrefix(out, []byte("\x02#!SILK_V3")) {
		out = out[1:]
	}
	path, err := r.writeDecodedToCache(sourcePath, out, "silk", cacheKey)
	if err != nil {
		return decryptFailed("voice", sourcePath, err.Error(), false)
	}
	return Info{
		Kind:       "voice",
		Status:     "resolved",
		Path:       path,
		SourcePath: sourcePath,
		Decoded:    false,
		Thumbnail:  false,
		Reason:     "voice_data exported as silk",
	}
}

func queryVoiceByServerID(dbPath string, serverID int64) ([]byte, bool) {
	return queryVoiceData(dbPath, "SELECT voice_data FROM VoiceInfo WHERE svr_id = ? AND length(voice_data) > 0 LIMIT 1;", serverID)
}

func queryVoiceByLocalID(dbPath string, chatNameID int64, localID int64) ([]byte, bool) {
	return queryVoiceData(dbPath, "SELECT voice_data FROM VoiceInfo WHERE chat_name_id = ? AND local_id = ? AND length(voice_data) > 0 LIMIT 1;", chatNameID, localID)
}

func queryVoiceData(dbPath string, query string, args ...any) ([]byte, bool) {
	db, err := sqlitedb.OpenReadOnly(context.Background(), dbPath)
	if err != nil {
		return nil, false
	}
	defer db.Close()
	var data []byte
	if err := db.QueryRowContext(context.Background(), query, args...).Scan(&data); err != nil || len(data) == 0 {
		return nil, false
	}
	return data, true
}

func queryChatNameID(dbPath string, username string) (int64, bool) {
	db, err := sqlitedb.OpenReadOnly(context.Background(), dbPath)
	if err != nil {
		return 0, false
	}
	defer db.Close()
	var id int64
	if err := db.QueryRowContext(context.Background(), "SELECT rowid FROM Name2Id WHERE user_name = ? LIMIT 1;", username).Scan(&id); err != nil {
		return 0, false
	}
	return id, id > 0
}
