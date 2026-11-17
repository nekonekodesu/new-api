package system_setting

import (
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

const (
	PathScanEnabledOptionKey       = "path_scan_setting.path_scan_enabled"
	PathScanWindowSecondsOptionKey = "path_scan_setting.path_scan_window_seconds"
	PathScanMaxRequestsOptionKey   = "path_scan_setting.path_scan_max_requests"
	PathScanBanDurationOptionKey   = "path_scan_setting.path_scan_ban_duration"
	PathScanBanDurationUnitKey     = "path_scan_setting.path_scan_ban_duration_unit"
	PathScanDirectPathsOptionKey   = "path_scan_setting.path_scan_direct_paths"
	PathScanCountedPathsOptionKey  = "path_scan_setting.path_scan_counted_paths"

	maxPathScanRuleLines  = 500
	maxPathScanRuleLength = 2048
)

type PathScanSetting struct {
	Enabled         bool   `json:"path_scan_enabled"`
	WindowSeconds   int    `json:"path_scan_window_seconds"`
	MaxRequests     int    `json:"path_scan_max_requests"`
	BanDuration     int    `json:"path_scan_ban_duration"`
	BanDurationUnit string `json:"path_scan_ban_duration_unit"`
	DirectPaths     string `json:"path_scan_direct_paths"`
	CountedPaths    string `json:"path_scan_counted_paths"`
}

type pathScanRuleKind uint8

const (
	pathScanRulePlain pathScanRuleKind = iota
	pathScanRuleWildcard
	pathScanRuleRegex
)

type compiledPathScanRule struct {
	kind    pathScanRuleKind
	raw     string
	pattern string
	re      *regexp.Regexp
}

type pathScanSnapshot struct {
	setting      PathScanSetting
	directRules  []compiledPathScanRule
	countedRules []compiledPathScanRule
}

var pathScanSetting = PathScanSetting{WindowSeconds: 10, MaxRequests: 20, BanDuration: 1, BanDurationUnit: "hour"}
var pathScanRuntime atomic.Pointer[pathScanSnapshot]

func init() {
	config.GlobalConfig.Register("path_scan_setting", &pathScanSetting)
	SetPathScanSetting(pathScanSetting)
}

func GetPathScanSetting() PathScanSetting {
	if snap := pathScanRuntime.Load(); snap != nil {
		return snap.setting
	}
	return normalizePathScanSetting(pathScanSetting)
}

func SetPathScanSetting(next PathScanSetting) {
	normalized := normalizePathScanSetting(next)
	directRules, _ := compilePathScanRules(normalized.DirectPaths, false)
	countedRules, _ := compilePathScanRules(normalized.CountedPaths, false)
	snap := &pathScanSnapshot{
		setting:      normalized,
		directRules:  directRules,
		countedRules: countedRules,
	}
	pathScanRuntime.Store(snap)
}

// MatchDirectPathScan checks whether rawPath matches any pre-compiled DirectPaths rule.
func MatchDirectPathScan(rawPath string) bool {
	snap := pathScanRuntime.Load()
	if snap == nil || len(snap.directRules) == 0 {
		return false
	}
	return matchCompiledRules(rawPath, snap.directRules)
}

// MatchCountedPathScan checks whether rawPath matches any pre-compiled CountedPaths rule.
func MatchCountedPathScan(rawPath string) bool {
	snap := pathScanRuntime.Load()
	if snap == nil || len(snap.countedRules) == 0 {
		return false
	}
	return matchCompiledRules(rawPath, snap.countedRules)
}

// MatchPathScanRules evaluates rawPath against an ad-hoc slice of rule lines.
func MatchPathScanRules(rawPath string, rules []string) bool {
	if len(rules) == 0 {
		return false
	}
	compiled := make([]compiledPathScanRule, 0, len(rules))
	for _, line := range rules {
		rule, ok, err := parsePathScanRule(line)
		if err == nil && ok {
			compiled = append(compiled, rule)
		}
	}
	return matchCompiledRules(rawPath, compiled)
}

func PathScanBanDuration(setting PathScanSetting) time.Duration {
	setting = normalizePathScanSetting(setting)
	unit := time.Second
	switch setting.BanDurationUnit {
	case "minute":
		unit = time.Minute
	case "hour":
		unit = time.Hour
	case "day":
		unit = 24 * time.Hour
	case "year":
		unit = 365 * 24 * time.Hour
	}
	if setting.BanDuration > int((1<<63-1)/int64(unit)) {
		return time.Duration(1<<63 - 1)
	}
	return time.Duration(setting.BanDuration) * unit
}

func normalizePathScanSetting(setting PathScanSetting) PathScanSetting {
	if setting.WindowSeconds <= 0 {
		setting.WindowSeconds = 10
	}
	if setting.MaxRequests <= 0 {
		setting.MaxRequests = 20
	}
	if setting.BanDuration <= 0 {
		setting.BanDuration = 1
	}
	switch setting.BanDurationUnit {
	case "second", "minute", "hour", "day", "year":
	default:
		setting.BanDurationUnit = "hour"
	}
	return setting
}

func PathList(raw string) []string {
	rules, _ := compilePathScanRules(raw, false)
	paths := make([]string, 0, len(rules))
	for _, r := range rules {
		paths = append(paths, r.raw)
	}
	return paths
}

func ValidatePathScanOption(key, value string) error {
	switch key {
	case PathScanEnabledOptionKey:
		if _, err := strconv.ParseBool(value); err != nil {
			return common.NewMessage("Path scan protection enabled must be true or false")
		}
	case PathScanWindowSecondsOptionKey, PathScanMaxRequestsOptionKey, PathScanBanDurationOptionKey:
		number, err := strconv.Atoi(value)
		if err != nil || number <= 0 {
			return common.NewMessage("Path scan protection values must be positive integers")
		}
		if key == PathScanWindowSecondsOptionKey && number > 86400 {
			return common.NewMessage("Path scan window cannot exceed one day")
		}
		if key == PathScanMaxRequestsOptionKey && number > 100000 {
			return common.NewMessage("Path scan request limit cannot exceed 100000")
		}
		if key == PathScanBanDurationOptionKey && number > 36500 {
			return common.NewMessage("Path scan ban duration cannot exceed 36500")
		}
	case PathScanBanDurationUnitKey:
		if value != "second" && value != "minute" && value != "hour" && value != "day" && value != "year" {
			return common.NewMessage("Path scan ban duration unit is invalid")
		}
	case PathScanDirectPathsOptionKey, PathScanCountedPathsOptionKey:
		if _, err := compilePathScanRules(value, true); err != nil {
			return err
		}
	}
	return nil
}

func compilePathScanRules(raw string, strict bool) ([]compiledPathScanRule, error) {
	var compiled []compiledPathScanRule
	lineCount := 0
	for rawLine := range strings.SplitSeq(raw, "\n") {
		rule, ok, err := parsePathScanRule(rawLine)
		if err != nil {
			if strict {
				return nil, err
			}
			continue
		}
		if !ok {
			continue
		}
		lineCount++
		if lineCount > maxPathScanRuleLines {
			if strict {
				return nil, common.NewMessage("Path scan rules cannot exceed 500 lines")
			}
			break
		}
		compiled = append(compiled, rule)
	}
	return compiled, nil
}

func parsePathScanRule(rawLine string) (compiledPathScanRule, bool, error) {
	line := strings.TrimSpace(rawLine)
	if line == "" {
		return compiledPathScanRule{}, false, nil
	}
	if len(line) > maxPathScanRuleLength {
		return compiledPathScanRule{}, false, common.NewMessage("Each path scan rule cannot exceed 2048 characters")
	}

	// 1. Explicit Nginx / regex prefixes: "~* <regex>", "~ <regex>", "regex:<regex>"
	if after, ok := strings.CutPrefix(line, "~*"); ok {
		expr := strings.TrimSpace(after)
		if expr == "" {
			return compiledPathScanRule{}, false, common.NewMessage("Invalid regular expression in path scan rules")
		}
		re, err := regexp.Compile("(?i)" + expr)
		if err != nil {
			return compiledPathScanRule{}, false, common.NewMessage("Invalid regular expression in path scan rules")
		}
		return compiledPathScanRule{kind: pathScanRuleRegex, raw: line, re: re}, true, nil
	}
	if after, ok := strings.CutPrefix(line, "~"); ok {
		expr := strings.TrimSpace(after)
		if expr == "" {
			return compiledPathScanRule{}, false, common.NewMessage("Invalid regular expression in path scan rules")
		}
		re, err := regexp.Compile(expr)
		if err != nil {
			return compiledPathScanRule{}, false, common.NewMessage("Invalid regular expression in path scan rules")
		}
		return compiledPathScanRule{kind: pathScanRuleRegex, raw: line, re: re}, true, nil
	}
	if len(line) >= 6 && strings.EqualFold(line[:6], "regex:") {
		expr := strings.TrimSpace(line[6:])
		if expr == "" {
			return compiledPathScanRule{}, false, common.NewMessage("Invalid regular expression in path scan rules")
		}
		re, err := regexp.Compile("(?i)" + expr)
		if err != nil {
			return compiledPathScanRule{}, false, common.NewMessage("Invalid regular expression in path scan rules")
		}
		return compiledPathScanRule{kind: pathScanRuleRegex, raw: line, re: re}, true, nil
	}

	// 2. Leading "/" or "*" without regex metacharacters -> Plain path or Wildcard (*) path rule
	if (strings.HasPrefix(line, "/") || strings.HasPrefix(line, "*")) && !strings.ContainsAny(line, "()[]|\\^$+{}") {
		if strings.ContainsAny(line, "?#") {
			return compiledPathScanRule{}, false, common.NewMessage("Each path scan rule must start with / (supports * wildcard) or be a valid regular expression")
		}
		normalized := line
		if strings.HasPrefix(normalized, "*") {
			normalized = "/" + normalized
		}
		for strings.Contains(normalized, "**") {
			normalized = strings.ReplaceAll(normalized, "**", "*")
		}
		lower := strings.ToLower(normalized)
		if strings.Contains(lower, "*") {
			return compiledPathScanRule{kind: pathScanRuleWildcard, raw: line, pattern: lower}, true, nil
		}
		if lower != "/" {
			lower = strings.TrimSuffix(lower, "/")
		}
		return compiledPathScanRule{kind: pathScanRulePlain, raw: line, pattern: lower}, true, nil
	}

	// 3. Slash-delimited JS regex ("/pattern/i" or "/^pattern$/") or raw regex expression
	if after, ok := strings.CutPrefix(line, "/"); ok && strings.ContainsAny(line, "()[]|\\^$+{}") {
		expr := line
		if inner, hasIFlag := strings.CutSuffix(after, "/i"); hasIFlag && inner != "" {
			expr = inner
		} else if inner, hasSlash := strings.CutSuffix(after, "/"); hasSlash && inner != "" &&
			(strings.HasPrefix(inner, "^") || strings.HasPrefix(inner, "\\") || strings.HasSuffix(inner, "$")) {
			expr = inner
		}
		re, err := regexp.Compile("(?i)" + expr)
		if err != nil {
			return compiledPathScanRule{}, false, common.NewMessage("Invalid regular expression in path scan rules")
		}
		return compiledPathScanRule{kind: pathScanRuleRegex, raw: line, re: re}, true, nil
	}

	if strings.ContainsAny(line, "()[]|\\^$+{}") || strings.Contains(line, ".*") {
		re, err := regexp.Compile("(?i)" + line)
		if err != nil {
			return compiledPathScanRule{}, false, common.NewMessage("Invalid regular expression in path scan rules")
		}
		return compiledPathScanRule{kind: pathScanRuleRegex, raw: line, re: re}, true, nil
	}

	return compiledPathScanRule{}, false, common.NewMessage("Each path scan rule must start with / (supports * wildcard) or be a valid regular expression")
}

func matchCompiledRules(rawPath string, rules []compiledPathScanRule) bool {
	if len(rules) == 0 {
		return false
	}
	cleanOrig, cleanSlashOrig, cleanLower, cleanSlashLower := normalizePathScanTargets(rawPath)
	for i := range rules {
		rule := &rules[i]
		switch rule.kind {
		case pathScanRulePlain:
			if rule.pattern == "/" {
				if cleanLower == "/" {
					return true
				}
				continue
			}
			if cleanLower == rule.pattern || strings.HasPrefix(cleanLower, rule.pattern+"/") {
				return true
			}
		case pathScanRuleWildcard:
			if matchWildcard(rule.pattern, cleanLower) {
				return true
			}
			if cleanSlashLower != cleanLower && matchWildcard(rule.pattern, cleanSlashLower) {
				return true
			}
		case pathScanRuleRegex:
			if rule.re.MatchString(cleanOrig) {
				return true
			}
			if cleanSlashOrig != cleanOrig && rule.re.MatchString(cleanSlashOrig) {
				return true
			}
		}
	}
	return false
}

func normalizePathScanTargets(rawPath string) (cleanOrig, cleanSlashOrig, cleanLower, cleanSlashLower string) {
	if rawPath == "" {
		rawPath = "/"
	}
	if !strings.HasPrefix(rawPath, "/") {
		rawPath = "/" + rawPath
	}
	hasTrailingSlash := len(rawPath) > 1 && strings.HasSuffix(rawPath, "/")
	cleanOrig = path.Clean(rawPath)
	if !strings.HasPrefix(cleanOrig, "/") {
		cleanOrig = "/" + cleanOrig
	}
	cleanSlashOrig = cleanOrig
	if hasTrailingSlash && cleanOrig != "/" {
		cleanSlashOrig = cleanOrig + "/"
	}
	cleanLower = strings.ToLower(cleanOrig)
	cleanSlashLower = strings.ToLower(cleanSlashOrig)
	return cleanOrig, cleanSlashOrig, cleanLower, cleanSlashLower
}

// matchWildcard performs non-recursive linear-time glob matching where '*' matches
// zero or more characters (including '/'). Both pattern and target are pre-lowercased.
func matchWildcard(pattern, target string) bool {
	pIdx, tIdx := 0, 0
	starIdx, matchIdx := -1, 0
	for tIdx < len(target) {
		if pIdx < len(pattern) && pattern[pIdx] == target[tIdx] {
			pIdx++
			tIdx++
		} else if pIdx < len(pattern) && pattern[pIdx] == '*' {
			starIdx = pIdx
			matchIdx = tIdx
			pIdx++
		} else if starIdx != -1 {
			pIdx = starIdx + 1
			matchIdx++
			tIdx = matchIdx
		} else {
			return false
		}
	}
	for pIdx < len(pattern) && pattern[pIdx] == '*' {
		pIdx++
	}
	return pIdx == len(pattern)
}
