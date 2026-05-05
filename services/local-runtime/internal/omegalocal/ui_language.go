package omegalocal

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

const uiLanguageSettingKey = "ui_language"

type uiLanguagePreference struct {
	Language  string `json:"language"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

func normalizeUILanguage(value string) string {
	normalized := strings.TrimSpace(value)
	switch normalized {
	case "zh-CN", "zh", "zh_CN":
		return "zh-CN"
	default:
		return "en"
	}
}

func (server *Server) currentUILanguage(ctx context.Context) string {
	record, err := server.Repo.GetSetting(ctx, uiLanguageSettingKey)
	if err != nil {
		return "en"
	}
	return normalizeUILanguage(text(record, "language"))
}

func (server *Server) getUILanguage(response http.ResponseWriter, request *http.Request) {
	record, err := server.Repo.GetSetting(request.Context(), uiLanguageSettingKey)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(response, http.StatusOK, uiLanguagePreference{Language: "en"})
		return
	}
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, uiLanguagePreference{
		Language:  normalizeUILanguage(text(record, "language")),
		UpdatedAt: text(record, "updatedAt"),
	})
}

func (server *Server) putUILanguage(response http.ResponseWriter, request *http.Request) {
	var payload uiLanguagePreference
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	record := map[string]any{
		"language":  normalizeUILanguage(payload.Language),
		"updatedAt": nowISO(),
	}
	if err := server.Repo.SetSetting(request.Context(), uiLanguageSettingKey, record); err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, uiLanguagePreference{
		Language:  text(record, "language"),
		UpdatedAt: text(record, "updatedAt"),
	})
}
