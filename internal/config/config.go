// Package config provides configuration from environment variables with validation.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config caches all configuration values on creation for fast access.
type Config struct {
	m3uSourceURL       string
	s3BucketName       string
	s3FilteredKey      string
	s3AllKey           string
	s3EndpointURL      string
	s3Region           string
	epgSourceURL       string
	s3EPGKey           string
	localEPGPath       string
	epgRetention       int
	outputDir          string
	categoriesFile     string
	dryRun             bool
	skipSSLVerify      bool
	probeSources       bool
	probeTimeout       time.Duration
	probeConcurrency   int
	maxChannelVariants int
}

// New reads all environment variables once and caches them.
func New() *Config {
	return &Config{
		m3uSourceURL:       os.Getenv("M3U_SOURCE_URL"),
		s3BucketName:       os.Getenv("S3_BUCKET_NAME"),
		s3FilteredKey:      envOrDefault("S3_OBJECT_KEY", "playlist.m3u"),
		s3AllKey:           "playlist-all.m3u",
		s3EndpointURL:      os.Getenv("S3_ENDPOINT_URL"),
		s3Region:           envOrDefault("S3_REGION", "us-east-1"),
		epgSourceURL:       os.Getenv("EPG_SOURCE_URL"),
		s3EPGKey:           os.Getenv("S3_EPG_KEY"),
		localEPGPath:       envOrDefault("LOCAL_EPG_PATH", "epg.xml.gz"),
		epgRetention:       envIntOrDefault("EPG_RETENTION_DAYS", 3),
		outputDir:          envOrDefault("OUTPUT_DIR", "output"),
		categoriesFile:     os.Getenv("CATEGORIES_FILE_PATH"),
		dryRun:             isTruthy(os.Getenv("DRY_RUN")),
		skipSSLVerify:      isTruthy(os.Getenv("SKIP_SSL_VERIFY")),
		probeSources:       isTruthy(os.Getenv("PROBE_SOURCES")),
		probeTimeout:       time.Duration(envIntOrDefault("PROBE_TIMEOUT_SECONDS", 5)) * time.Second,
		probeConcurrency:   envIntOrDefault("PROBE_CONCURRENCY", 20),
		maxChannelVariants: envIntClampMax("MAX_CHANNEL_VARIANTS", 1, 5),
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

// envIntClampMax returns the env int (or defaultVal), clamped to a maximum.
func envIntClampMax(key string, defaultVal, max int) int {
	n := envIntOrDefault(key, defaultVal)
	if n > max {
		return max
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

// ProbeSources returns whether to probe stream URL availability and keep only
// working sources (option C). Disabled by default.
func (c *Config) ProbeSources() bool { return c.probeSources }

// ProbeTimeout returns the per-request timeout used when probing sources.
func (c *Config) ProbeTimeout() time.Duration { return c.probeTimeout }

// ProbeConcurrency returns the number of parallel probe requests.
func (c *Config) ProbeConcurrency() int { return c.probeConcurrency }

// MaxChannelVariants returns how many variants to keep per channel (1-5).
func (c *Config) MaxChannelVariants() int { return c.maxChannelVariants }

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
	"XXX",
	// Религия
	"Религия",
	"Религиозные",
	// Поддержка / INFO
	"💲💲💲Поддержи Проект💲💲💲", // leading emojis — normalization only strips trailing
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
	"MavTV ⭐️",         // trailing emoji — normalization strips it
	"aleks-u-romki* 😊", // trailing emoji — normalization strips it
	"Play-x",
	// Кино и сериалы (bold unicode)
	"𝐊𝐢𝐧𝐨",
	"𝕂иℍ𝕠",
	// Кино и Спорт (фильтруются целиком; см. также CategoriesToRemoveByKeyword
	// для каналов, реклассифицируемых в эти категории по имени)
	"Кино",
	"Спорт",
	// Региональные категории
	"РОССИЯ+",

	// Другие
	"Гранд",
}

// CategoriesToRemoveSubstring lists substrings to match against group-title (case-insensitive).
// Any category whose name contains one of these substrings will be filtered out.
var CategoriesToRemoveSubstring = []string{

	// Adult / 18+
	"xxx",
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
	"tvz", "tv s", "tvzotak", "mavtv", "aleks", "play-x",
	// Служебные (bold unicode: 𝕋𝕧ℤ𝕒𝕋𝕒𝕜)
	"𝕋𝕧ℤ",
	// Поддержка
	"поддержи",
	// Страны / Регионы. European countries are intentionally NOT listed here
	// (they are kept on the allow-list, see AllowedCategories); this list only
	// filters out non-European or unwanted-country categories.
	"азербайджан", "алжир",
	"аргентин", "армени", "афганистан",
	"боливи", "бразили",
	"казахстан", "кыргызстан",
	"вьетнам", "гаити",
	"гватемал", "гондурас",
	"грузия", "доминикан", "египет",
	"израиль", "индонези", "иордан",
	"ирак", "иран",
	"йемен", "катар", "кени", "коре",
	"коста-рика", "лаос", "ливан",
	"малайзи", "мальдив", "марок", "мексик",
	"оаэ", "оман",
	"пакистан", "перу",
	"руанд",
	"сальвадор", "сан-марин", "саудовск", "северная македони",
	"сомали",
	"таджикистан", "таиланд", "тайвань",
	"туркменистан", "турци", "узбекистан",
	"чили",
	"шри-ланка",
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

// CategoryAliases maps source group-title values (provider-specific spellings,
// emoji variants, duplicate country names) to canonical names. Applied before
// the allow-list check so known-good variants collapse into a single category
// (e.g. РЕГИОНАЛЬНЫЕ → Региональные, NEWS 🆕 → Новости).
var CategoryAliases = map[string]string{
	// Региональные
	"РЕГИОНАЛЬНЫЕ":              "Региональные",
	"Основные (региональные)":   "Региональные",
	"Популярные (региональные)": "Региональные",
	// Эфирные
	"ЭФИРНЫЕ ⓵":    "Эфирные",
	"ЭФИРНЫЕ":      "Эфирные",
	"ЭФИРНЫЕ Int.": "Эфирные",
	// Контентные
	"ПОЗНАВАТЕЛЬНЫЕ":    "Познавательные",
	"РАЗВЛЕКАТЕЛЬНЫЕ":   "Развлекательные",
	"NEWS 🆕":            "Новости",
	"НОВОСТИ":           "Новости",
	"Новостные":         "Новости",
	"Информационные":    "Новости",
	"ПУТЕШЕСТВИЯ":       "Путешествия",
	"ПРИРОДА":           "Природа",
	"ХОББИ И УВЛЕЧЕНИЯ": "Хобби",
	"РОССИЙСКИЕ":        "Российские",
	"*MUZICA":           "Музыка",
	// Страны
	"США | USA":                       "США",
	"Германия | Germany":              "Германия",
	"Канада | Canada":                 "Канада",
	"Индия | India":                   "Индия",
	"Швеция | Sweden":                 "Швеция",
	"Великобритания | United Kingdom": "Великобритания",
	"БЕЛАРУСЬ":                        "Беларусь",
	"Беларусь | Беларускія":           "Беларусь",
	"Австралия | Australia":           "Австралия",
	"Латвия | Latvia":                 "Латвия",
	"Литва | Lithuania":               "Литва",
	"Хорватия | Croatia":              "Хорватия",
	"Эстония | Estonia":               "Эстония",
	"Чехия | Czech Republic":          "Чехия",
	"Дания | Denmark":                 "Дания",
	"Европа | Europe":                 "Европа",
	"Объединенные Арабские Эмираты":   "ОАЭ",
	// Европейские страны (варианты с "|" и флагами)
	"Италия | Italy":           "Италия",
	"Румыния | Romania":        "Румыния",
	"Испания | Spain":          "Испания",
	"Польша | Poland":          "Польша",
	"Франция | France":         "Франция",
	"Молдавия | Moldovenească": "Молдавия",
	"Португалия | Portugal":    "Португалия",
	"Болгария | Bulgaria":      "Болгария",
	"Нидерланды | Netherlands": "Нидерланды",
	"Норвегия | Norway":        "Норвегия",
	"Словакия | Slovakia":      "Словакия",
	"Финляндия | Finland":      "Финляндия",
	"Албания 🇦🇱":               "Албания",
	"Австрия 🇦🇹":               "Австрия",
	"Бельгия 🇧🇪":               "Бельгия",
	"Болгария 🇧🇬":              "Болгария",
	"Венгрия 🇭🇺":               "Венгрия",
	"Греция 🇬🇷":                "Греция",
	"Ирландия 🇮🇪":              "Ирландия",
	"Испания 🇪🇸":               "Испания",
	"Италия 🇮🇹":                "Италия",
	"Люксембург 🇱🇺":            "Люксембург",
	"Молдавия 🇲🇩":              "Молдавия",
	"Нидерланды 🇳🇱":            "Нидерланды",
	"Норвегия 🇳🇴":              "Норвегия",
	"Польша 🇵🇱":                "Польша",
	"Португалия 🇵🇹":            "Португалия",
	"Румыния 🇷🇴":               "Румыния",
	"Сербия 🇷🇸":                "Сербия",
	"Словакия 🇸🇰":              "Словакия",
	"Финляндия 🇫🇮":             "Финляндия",
	"Франция 🇫🇷":               "Франция",
	"Швейцария 🇨🇭":             "Швейцария",
	"Арабские | عربي":          "Арабские",
	// Качество
	"4K VIDEO":       "4K",
	"4K VIDEO (VPN)": "4K",
	// Провайдерские дубли (одна и та же категория в разных написаниях)
	"Onair8k2*":      "Onair8k2",
	"С сайтов (VPN)": "С сайтов",
	"Квант-Телеком (VPN 🇷🇺)": "Квант-Телеком",
	"↕️ Торрент ТВ ↕":        "Торрент ТВ",
}

// AllowedCategories is the allow-list of group-titles kept as-is after
// normalization. Channels in any other category are moved to FallbackCategory.
// This keeps only quality content categories while preserving every channel.
var AllowedCategories = []string{
	// Контентные. «Кино» is intentionally NOT here: channels matching cinema
	// keywords are removed entirely (see CategoriesToRemoveByKeyword), not kept.
	"Познавательные", "Развлекательные", "Новости", "Детские",
	"Музыка", "Путешествия", "Природа", "Хобби",
	// География / вещание
	"Региональные", "Эфирные", "Российские",
	// Общие
	"Популярные", "Общие", "Основные", "4K",
	// Страны
	"Германия", "США", "Канада", "Индия", "Швеция", "Великобритания", "Беларусь",
	"Австралия", "Латвия", "Литва", "Хорватия", "Эстония", "Китай", "Чехия",
	"Дания", "Новая Зеландия", "Европа", "ОАЭ", "Арабские",
	// Европейские страны (добавлены — каналы уже есть в источниках)
	"Италия", "Румыния", "Испания", "Польша", "Франция", "Молдавия",
	"Португалия", "Болгария", "Нидерланды", "Греция", "Словакия", "Бельгия",
	"Норвегия", "Сербия", "Австрия", "Венгрия", "Швейцария", "Финляндия",
	"Ирландия", "Албания", "Люксембург",
}

// FallbackCategory is the group-title assigned to channels whose category is
// not on the allow-list.
const FallbackCategory = "Основные"

// AllowedCategorySet returns the allow-list as a set for O(1) lookup.
func AllowedCategorySet() map[string]bool {
	set := make(map[string]bool, len(AllowedCategories))
	for _, c := range AllowedCategories {
		set[c] = true
	}
	return set
}

// CategoryKeywords maps a canonical category to channel-name keywords that
// reliably indicate its genre. Used to reclassify channels that fell into
// FallbackCategory after allow-list normalization: a channel whose name
// contains a keyword moves to that category instead of staying in "Основные".
// Matching is case-insensitive on the emoji-stripped name; longer keywords
// take priority over shorter ones, and category priority follows slice order
// (more specific genres first). Keywords are deliberately conservative — only
// unambiguous genre markers, so nothing is misclassified.
var CategoryKeywords = map[string][]string{
	// Спорт — most specific first.
	"Спорт": {
		"футбол", "football", "хоккей", "hockey", "теннис", "tennis",
		"бокс", "boxing", "биатлон", "баскетбол", "волейбол", "реслинг",
		"wrestling", "eurosport", "viasat sport", "матч", "формула", "racing",
		"гонки", "автоспорт", "ufc", "mma", "nhl", "nba", "кхл", "sport",
		"спорт", "спорт 1", "спорт 2", "хоккей кхл",
	},
	// Детские.
	"Детские": {
		"мульт", "cartoon", "карусель", "karusel", "disney", "nickelodeon",
		"nick jr", "boomerang", "gulli", "tiji", "jimjam", "da vinci",
		"малыш", "baby tv", "детск", "детям", "kids", "солнышко", "ленд",
	},
	// Кино.
	"Кино": {
		"кино", "cinema", "movie", "фильм", "сериал", "tv1000", "hbo",
		"кинопоказ", "kinopokaz", "кинопоиск", "kinopoisk", "amc",
		"дом кино", "kino", "драма", "drama", "комеди", "comedy",
		"thriller", "триллер", "боевик", "детектив", "кинопремьера",
	},
	// Новости.
	"Новости": {
		"новости", "новост", "news", "russia today", "russia 24", "россия 24",
		"мир 24", "rbc", "рбк", "cnn", "euronews", "al jazeera",
		"аль джазира", "life news", "24 news", "tv centr", "тв центр", "дождь",
	},
	// Музыка.
	"Музыка": {
		"музык", "music", "муз тв", "muz tv", "mtv", "vh1", "mcm", "hit",
		"шансон", "ретро", "rock", "рок", "pop", "поп", "dance", "данс",
		"радио", "radio", "мелоди", "эстрад", "классик", "джаз", "jazz",
		"опера", "этно", "голос", "меломан",
	},
	// Познавательные.
	"Познавательные": {
		"discovery", "дискавери", "viasat", "nat geo", "национальная географи",
		"history", "история", "наука", "science", "документал", "documentary",
		"познават", "cosmos", "космос", "universe", "вселенная", "образование",
		"education", "умный", "smart tv", "зоо", "зоопарк", "zoo tv",
	},
	// Путешествия.
	"Путешествия": {
		"travel", "путешеств", "вояж", "adventure", "приключ", "trip",
	},
	// Природа.
	"Природа": {
		"nature", "природа", "wildlife", "животн", "animal planet",
		"планета животных", "дикая",
	},
	// Хобби.
	"Хобби": {
		"хобби", "hobby", "рыбал", "охота", "огород", "кулинар", "cooking",
		"дача", "ремонт", "авто", "auto",
	},
	// Развлекательные.
	"Развлекательные": {
		"развлекат", "entertainment", "тнт", "стс", "пятница", "тв3", "супер",
		"перец", "рен тв", "ren tv", "домашний", "звезда", "ю", "че", "2х2",
		"2x2", "тв3", "пятница!",
	},
}

// CategoriesToRemoveByKeyword lists categories whose channels must be REMOVED
// entirely (not reclassified) when ClassifyFallbackCategories matches them by
// name keyword. Used to filter out a genre category after the allow-list pass,
// e.g. «Кино»: channels whose name contains cinema keywords (HBO, AMC,
// "кино", "фильм"...) are dropped instead of staying in the playlist.
var CategoriesToRemoveByKeyword = []string{"Кино", "Спорт"}

// CategoriesToRemoveByKeywordSet returns the categories to remove as a set for
// O(1) lookup.
func CategoriesToRemoveByKeywordSet() map[string]bool {
	set := make(map[string]bool, len(CategoriesToRemoveByKeyword))
	for _, c := range CategoriesToRemoveByKeyword {
		set[c] = true
	}
	return set
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

	if c.epgSourceURL != "" {
		lower := strings.ToLower(c.epgSourceURL)
		for _, p := range placeholderPatterns {
			if strings.Contains(lower, p) {
				errors = append(errors, "EPG_SOURCE_URL appears to be a placeholder. Please set a valid URL")
				break
			}
		}
		if len(errors) == 0 {
			urls := strings.Split(c.epgSourceURL, ",")
			hasValid := false
			for _, u := range urls {
				u = strings.TrimSpace(u)
				if u != "" {
					hasValid = true
					if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
						errors = append(errors, fmt.Sprintf("EPG_SOURCE_URL contains invalid URL: %s", u))
					}
				}
			}
			if !hasValid {
				errors = append(errors, "EPG_SOURCE_URL must contain at least one valid HTTP/HTTPS URL")
			}
		}
	}

	if !c.dryRun && c.s3BucketName == "" {
		errors = append(errors, "S3_BUCKET_NAME must be specified")
	} else if c.s3BucketName != "" && (len(c.s3BucketName) < 3 || len(c.s3BucketName) > 63) {
		errors = append(errors, "S3_BUCKET_NAME must be between 3 and 63 characters")
	}

	if c.s3FilteredKey == "" || strings.Contains(c.s3FilteredKey, "..") || strings.HasPrefix(c.s3FilteredKey, "/") {
		errors = append(errors, "S3_OBJECT_KEY must not contain '..' or start with '/'")
	}

	if c.epgSourceURL != "" && (c.s3EPGKey == "" || strings.Contains(c.s3EPGKey, "..") || strings.HasPrefix(c.s3EPGKey, "/")) {
		errors = append(errors, "S3_EPG_KEY must not contain '..' or start with '/'")
	}

	if !c.dryRun && c.s3EndpointURL == "" {
		errors = append(errors, "S3_ENDPOINT_URL must be specified")
	} else if c.s3EndpointURL != "" && !strings.HasPrefix(c.s3EndpointURL, "http://") && !strings.HasPrefix(c.s3EndpointURL, "https://") {
		errors = append(errors, "S3_ENDPOINT_URL must be a valid HTTP/HTTPS URL")
	} else {
		parsed, err := url.Parse(c.s3EndpointURL)
		if err == nil && parsed.User != nil {
			errors = append(errors, "S3_ENDPOINT_URL should not contain credentials in the URL")
		}
	}

	if !c.dryRun && c.s3Region == "" {
		errors = append(errors, "S3_REGION must be specified")
	}

	return errors
}
