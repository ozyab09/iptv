// Package config provides configuration from environment variables with validation.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Config caches all configuration values on creation for fast access.
type Config struct {
	m3uSourceURL    string
	s3BucketName    string
	s3FilteredKey   string
	s3AllKey        string
	s3EndpointURL   string
	s3Region        string
	epgSourceURL    string
	s3EPGKey        string
	localEPGPath    string
	epgRetention    int
	outputDir       string
	categoriesFile  string
	dryRun          bool
	skipSSLVerify   bool
}

// New reads all environment variables once and caches them.
func New() *Config {
	return &Config{
		m3uSourceURL:  os.Getenv("M3U_SOURCE_URL"),
		s3BucketName:  os.Getenv("S3_BUCKET_NAME"),
		s3FilteredKey: envOrDefault("S3_OBJECT_KEY", "playlist.m3u"),
		s3AllKey:      "playlist-all.m3u",
		s3EndpointURL: os.Getenv("S3_ENDPOINT_URL"),
		s3Region:      envOrDefault("S3_REGION", "us-east-1"),
		epgSourceURL:  os.Getenv("EPG_SOURCE_URL"),
		s3EPGKey:      os.Getenv("S3_EPG_KEY"),
		localEPGPath:  envOrDefault("LOCAL_EPG_PATH", "epg.xml.gz"),
		epgRetention:  envIntOrDefault("EPG_RETENTION_DAYS", 3),
		outputDir:     envOrDefault("OUTPUT_DIR", "output"),
		categoriesFile: os.Getenv("CATEGORIES_FILE_PATH"),
		dryRun:        isTruthy(os.Getenv("DRY_RUN")),
		skipSSLVerify: isTruthy(os.Getenv("SKIP_SSL_VERIFY")),
	}
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envIntOrDefault(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(val)
	if err != nil || n < 1 {
		return defaultVal
	}
	return n
}

func isTruthy(val string) bool {
	lower := strings.ToLower(val)
	return lower == "true" || val == "1" || lower == "yes" || lower == "on"
}

// M3USourceURL returns the M3U source URL.
func (c *Config) M3USourceURL() string { return c.m3uSourceURL }

// S3DefaultBucketName returns the S3 bucket name.
func (c *Config) S3DefaultBucketName() string { return c.s3BucketName }

// S3FilteredPlaylistKey returns the S3 key for the filtered playlist.
func (c *Config) S3FilteredPlaylistKey() string { return c.s3FilteredKey }

// S3AllCategoriesPlaylistKey returns the S3 key for the unfiltered playlist.
func (c *Config) S3AllCategoriesPlaylistKey() string { return c.s3AllKey }

// S3EndpointURL returns the S3-compatible endpoint URL.
func (c *Config) S3EndpointURL() string { return c.s3EndpointURL }

// S3Region returns the S3 region.
func (c *Config) S3Region() string { return c.s3Region }

// EPGSourceURL returns the EPG XML source URL.
func (c *Config) EPGSourceURL() string { return c.epgSourceURL }

// S3EPGKey returns the S3 key for the EPG file.
func (c *Config) S3EPGKey() string { return c.s3EPGKey }

// SkipSSLVerify returns whether to skip SSL certificate verification.
func (c *Config) SkipSSLVerify() bool { return c.skipSSLVerify }

// MaxM3UFileSize is the maximum M3U download size (100 MB).
const MaxM3UFileSize = 100 * 1024 * 1024

// MaxEPGFileSize is the maximum EPG download size (500 MB).
const MaxEPGFileSize = 500 * 1024 * 1024

// EPGRetentionDays returns how many days of EPG data to keep.
func (c *Config) EPGRetentionDays() int { return c.epgRetention }

// LocalFilteredPlaylistPath returns the local filename for the filtered playlist.
func (c *Config) LocalFilteredPlaylistPath() string { return c.s3FilteredKey }

// LocalAllCategoriesPlaylistPath returns the local filename for the unfiltered playlist.
func (c *Config) LocalAllCategoriesPlaylistPath() string {
	if idx := strings.LastIndex(c.s3FilteredKey, "."); idx >= 0 {
		return c.s3FilteredKey[:idx] + "-all" + c.s3FilteredKey[idx:]
	}
	return c.s3FilteredKey + "-all"
}

// LocalEPGPath returns the local filename for the downloaded EPG.
func (c *Config) LocalEPGPath() string { return c.localEPGPath }

// LocalFilteredEPGPath returns the local filename for the filtered EPG.
func (c *Config) LocalFilteredEPGPath() string {
	if idx := strings.LastIndex(c.s3EPGKey, "."); idx >= 0 {
		return c.s3EPGKey[:idx] + "-filtered" + c.s3EPGKey[idx:]
	}
	return c.s3EPGKey + "-filtered"
}

// EnsureOutputDir creates the output directory if it doesn't exist.
func (c *Config) EnsureOutputDir() error {
	return os.MkdirAll(c.outputDir, 0755)
}

// DryRun returns true if dry-run mode is enabled.
func (c *Config) DryRun() bool { return c.dryRun }

// OutputDir returns the local output directory.
func (c *Config) OutputDir() string { return c.outputDir }

// CategoriesFilePath returns the optional path to categories.txt.
func (c *Config) CategoriesFilePath() string { return c.categoriesFile }

// BuildCustomEPGURL constructs the public URL for the EPG file in S3.
func (c *Config) BuildCustomEPGURL() string {
	parsed, err := url.Parse(c.s3EndpointURL)
	if err != nil {
		hostPart := c.s3EndpointURL
		if idx := strings.Index(c.s3EndpointURL, "://"); idx >= 0 {
			hostPart = c.s3EndpointURL[idx+3:]
		}
		return fmt.Sprintf("https://%s.%s/%s", c.s3BucketName, hostPart, c.s3EPGKey)
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return fmt.Sprintf("%s://%s%s/%s/%s", parsed.Scheme, parsed.Host, strings.TrimRight(parsed.Path, "/"), c.s3BucketName, c.s3EPGKey)
	}
	return fmt.Sprintf("%s://%s.%s/%s", parsed.Scheme, c.s3BucketName, parsed.Host, c.s3EPGKey)
}

// CategoriesToRemove is the deny-list of channel groups to filter out (exact match, case-insensitive).
// Only normalized variants are kept — normalization strips leading numbers and trailing emojis
// before matching, so the original emojified variants would never match.
var CategoriesToRemove = []string{
	// Adult / 18+
	"Взрослые",
	"Adult (18+)",
	"XXX (18+)",
	// Религия
	"Религия",
	"Религиозные",
	// Поддержка / INFO
	"💲💲💲Поддержи Проект💲💲💲",   // leading emojis — normalization only strips trailing
	"🔺 INFO",
	// АнтиРоссия / Украина
	"АнтиРОССИЙСКИЕ",
	"𝕐𝕜𝕡𝕒Їℍ𝕒",
	"Українські",
	"Украина",
	"Наш Нет",
	// Спорт (bold unicode)
	"ℂп𝕠𝕡т",
	"186",
	// Музыка (bold unicode)
	"РАДИО ТВ",
	"𝕄𝕦𝕤𝕚𝕔",
	// Служебные / Тестовые (bold unicode)
	"𝕋𝕧ℤ𝕒𝕋𝕒𝕜",
	"32",
	"TVS",
	"TvZaTak",
	"MavTV ⭐️",                    // trailing emoji — normalization strips it
	"aleks-u-romki* 😊",            // trailing emoji — normalization strips it
	// Кино и сериалы (bold unicode)
	"𝐊𝐢𝐧𝐨",
	"𝕂иℍ𝕠",
	// Региональные категории
	"РОССИЯ+",

	// Другие
	"Гранд",
}

// CategoriesToRemoveSubstring lists substrings to match against group-title (case-insensitive).
// Any category whose name contains one of these substrings will be filtered out.
var CategoriesToRemoveSubstring = []string{

	// Спорт (обычный текст)
	"спорт", "sport", "матч",
	// Спорт (bold unicode: ℂп𝕠𝕡т)
	"п𝕠𝕡т",
	// Детские
	"детск", "мульт",
	// Музыка
	"радио тв", "музык", "dance", "retro", "bridge",
	"trace", "mtv", "record", "dfm",
	// Музыка (bold unicode: 𝕄𝕦𝕤𝕚𝕔, 145.32)
	"𝕤𝕚𝕔",
	// Религия
	"религи",
	// Relax
	"relax",
	// Мода / Магазины
	"мода", "телемагаз",
	// АнтиРоссия / Украина
	"антиросс", "україн", "украина", "наш нет",
	// Служебные / Тестовые
	"rutube", "cinerama", "watcher", "flussonic",
	"ngenix", "internet42", "ushba", "sewv",
	"tvz", "tv s", "tvzotak", "mavtv", "aleks",
	// Служебные (bold unicode: 𝕋𝕧ℤ𝕒𝕋𝕒𝕜)
	"𝕋𝕧ℤ",
	// Поддержка
	"поддержи",
	// Страны / Регионы
	"австри", "азербайджан", "албан", "алжир",
	"аргентин", "армени", "афганистан",
	"бельги", "болгари", "боливи", "бразили",
	"казахстан", "кыргызстан",
	"венгри", "вьетнам", "гаити",
	"гватемал", "гондурас", "греци",
	"грузия", "доминикан", "египет",
	"израиль", "индонези", "иордан",
	"ирак", "иран", "ирланди", "испан", "итали",
	"йемен", "катар", "кени", "коре",
	"коста-рика", "лаос", "ливан",
	"люксембург", "малайзи", "мальдив", "марок", "мексик", "молдав",
	"нидерланд", "норвег",
	"оаэ", "оман",
	"пакистан", "перу", "польш", "португал",
	"руанд", "румын",
	"сальвадор", "сан-марин", "саудовск", "северная македони",
	"серби", "словаки", "сомали",
	"таджикистан", "таиланд", "тайвань",
	"туркменистан", "турци", "узбекистан",
	"финлянди", "франци",
	"чили",
	"швейцари", "шри-ланка",
	"эквадор", "эфиопи", "япони",
	// Кино / Сериалы / Шоу (исключаемые категории)
	"кино", "девяностые", "кинозал", "киноstream",
	"сериал", "криминальная", "kinowalk",
	"екб", "kino", "viju",
	"тайны", "уральские",
	// TV шоу / Сериалы (конкретные проекты)
	"твоё тв", "itv.uz", "catcast",
	"домашний арест", "мир! дружба! жвачка",
	"полицейский с рублёвки", "прощание",
	"следствие вели", "слово пацана", "советские артисты",
}

// ChannelNamesToExclude lists channels removed by name substring match (case-insensitive).
var ChannelNamesToExclude = []string{
	"Fashion",
	"СПАС",
	"Три ангела",
	"ЛДПР",
	"UA",
	"Sports",
}

// EPGExcludedCategories lists EPG categories to exclude from the output.
var EPGExcludedCategories = []string{"Кино"}

// EPGExcludedChannelIDs lists specific EPG channel IDs to exclude.
var EPGExcludedChannelIDs = []string{
	"2745", "6170", "6168", "7553", "6171", "9228", "7552",
	"4729", "7594", "7595", "9233", "8822", "8817", "2438",
	"8811", "6848", "9025", "153", "66", "2760", "494",
	"6135", "9303", "5387", "2420", "2239", "9183", "774",
	"810", "6419",
}

// Validate checks all required configuration and returns a list of errors.
func (c *Config) Validate() []string {
	var errors []string
	placeholderPatterns := []string{"your-", "your_provider", "your-epg-provider"}

	if c.m3uSourceURL == "" {
		errors = append(errors, "M3U_SOURCE_URL must be specified")
	} else {
		lower := strings.ToLower(c.m3uSourceURL)
		for _, p := range placeholderPatterns {
			if strings.Contains(lower, p) {
				errors = append(errors, "M3U_SOURCE_URL appears to be a placeholder. Please set a valid URL")
				break
			}
		}
		if len(errors) == 0 {
			urls := strings.Split(c.m3uSourceURL, ",")
			hasValid := false
			for _, u := range urls {
				u = strings.TrimSpace(u)
				if u != "" {
					hasValid = true
					if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
						errors = append(errors, fmt.Sprintf("M3U_SOURCE_URL contains invalid URL: %s", u))
					}
				}
			}
			if !hasValid {
				errors = append(errors, "M3U_SOURCE_URL must contain at least one valid HTTP/HTTPS URL")
			}
		}
	}

	if c.epgSourceURL == "" {
		errors = append(errors, "EPG_SOURCE_URL must be specified")
	} else {
		lower := strings.ToLower(c.epgSourceURL)
		for _, p := range placeholderPatterns {
			if strings.Contains(lower, p) {
				errors = append(errors, "EPG_SOURCE_URL appears to be a placeholder. Please set a valid URL")
				break
			}
		}
		if len(errors) == 0 && !strings.HasPrefix(c.epgSourceURL, "http://") && !strings.HasPrefix(c.epgSourceURL, "https://") {
			errors = append(errors, "EPG_SOURCE_URL must be a valid HTTP/HTTPS URL")
		}
	}

	if c.s3BucketName == "" {
		errors = append(errors, "S3_BUCKET_NAME must be specified")
	} else if len(c.s3BucketName) < 3 || len(c.s3BucketName) > 63 {
		errors = append(errors, "S3_BUCKET_NAME must be between 3 and 63 characters")
	}

	if c.s3FilteredKey == "" || strings.Contains(c.s3FilteredKey, "..") || strings.HasPrefix(c.s3FilteredKey, "/") {
		errors = append(errors, "S3_OBJECT_KEY must not contain '..' or start with '/'")
	}

	if c.s3EPGKey == "" || strings.Contains(c.s3EPGKey, "..") || strings.HasPrefix(c.s3EPGKey, "/") {
		errors = append(errors, "S3_EPG_KEY must not contain '..' or start with '/'")
	}

	if c.s3EndpointURL == "" {
		errors = append(errors, "S3_ENDPOINT_URL must be specified")
	} else if !strings.HasPrefix(c.s3EndpointURL, "http://") && !strings.HasPrefix(c.s3EndpointURL, "https://") {
		errors = append(errors, "S3_ENDPOINT_URL must be a valid HTTP/HTTPS URL")
	} else {
		parsed, err := url.Parse(c.s3EndpointURL)
		if err == nil && strings.Contains(parsed.Host, "@") {
			errors = append(errors, "S3_ENDPOINT_URL should not contain credentials in the URL")
		}
	}

	if c.s3Region == "" {
		errors = append(errors, "S3_REGION must be specified")
	}

	return errors
}
