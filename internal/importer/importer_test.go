package importer

import "testing"

func TestParseLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		want    Record
		wantErr bool
	}{
		{
			name: "standard 6-field row",
			line: "tv/Show/S07E05.mkv|1362693441|1709383843|2026-05-25|Backup_2TB_Drive_3|UGREEN_Drive_1_12TB",
			want: Record{
				RelPath:     "tv/Show/S07E05.mkv",
				Size:        1362693441,
				Mtime:       1709383843,
				BackupDate:  "2026-05-25",
				DestLabel:   "Backup_2TB_Drive_3",
				SourceLabel: "UGREEN_Drive_1_12TB",
			},
		},
		{
			name: "unicode path",
			line: "tv/Show/S07E20 - Kill ‘Em All.srt|45973|1779704680|2026-05-25|Backup_2TB_Drive_3|UGREEN_Drive_1_12TB",
			want: Record{
				RelPath:     "tv/Show/S07E20 - Kill ‘Em All.srt",
				Size:        45973,
				Mtime:       1779704680,
				BackupDate:  "2026-05-25",
				DestLabel:   "Backup_2TB_Drive_3",
				SourceLabel: "UGREEN_Drive_1_12TB",
			},
		},
		{
			name: "pipe in rel_path is preserved (split from the right)",
			line: "movies/Face|Off (1997)/Face|Off.mkv|123|456|2026-05-25|Dest|Src",
			want: Record{
				RelPath:     "movies/Face|Off (1997)/Face|Off.mkv",
				Size:        123,
				Mtime:       456,
				BackupDate:  "2026-05-25",
				DestLabel:   "Dest",
				SourceLabel: "Src",
			},
		},
		{
			name: "legacy 5-field row without source label",
			line: "music/Album/track.flac|999|1000|2024-01-02|OldDrive",
			want: Record{
				RelPath:    "music/Album/track.flac",
				Size:       999,
				Mtime:      1000,
				BackupDate: "2024-01-02",
				DestLabel:  "OldDrive",
			},
		},
		{name: "too few fields", line: "a|b|c", wantErr: true},
		{name: "non-numeric size", line: "tv/x.mkv|big|123|2026-05-25|Dest|Src", wantErr: true},
		{name: "non-numeric mtime", line: "tv/x.mkv|123|nope|2026-05-25|Dest|Src", wantErr: true},
		{name: "empty dest label", line: "tv/x.mkv|123|456|2026-05-25||Src", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLine(tt.line)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got none (parsed %+v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ParseLine() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseBackupDate(t *testing.T) {
	if got := parseBackupDate("2026-05-25"); got != 1779667200 {
		t.Errorf("parseBackupDate(2026-05-25) = %d, want 1779667200", got)
	}
	if got := parseBackupDate("not-a-date"); got != 0 {
		t.Errorf("parseBackupDate(invalid) = %d, want 0", got)
	}
}
