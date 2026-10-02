package importer

import (
	"bufio"
	"io"
)

// ndjsonScanner — построчное чтение NDJSON (docs/plan/04 §4): одна JSON-строка —
// одна сущность; построчный разбор без загрузки файла целиком (большие
// объёмы), конкатенация файлов и частичный догрузочный импорт. Пустые
// строки и строки из пробелов пропускаются; комментариев нет.
type ndjsonScanner struct {
	sc    *bufio.Scanner
	line  int // номер последней прочитанной строки
	eof   bool
	empty bool // файл без единой сущности
}

func newNDJSONScanner(r io.Reader) *ndjsonScanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024) // до 16 МБ на строку
	return &ndjsonScanner{sc: sc, empty: true}
}

// next возвращает следующую непустую строку файла с её номером; ok = false
// — файл исчерпан. Ошибки чтения возвращает err.
func (s *ndjsonScanner) next() (line []byte, number int, ok bool, err error) {
	for s.sc.Scan() {
		s.line++
		if isEmptyLine(s.sc.Bytes()) {
			continue
		}
		s.empty = false
		return s.sc.Bytes(), s.line, true, nil
	}
	s.eof = true
	return nil, s.line, false, s.sc.Err()
}

func isEmptyLine(line []byte) bool {
	for _, b := range line {
		switch b {
		case ' ', '\t', '\r', '\n':
			continue
		}
		return false
	}
	return true
}
