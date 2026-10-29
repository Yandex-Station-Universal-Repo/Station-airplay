> Историческая запись: пакет 1.4.10 устанавливался только на Lite 2 192.168.0.196
> и заменён 1.4.11–1.4.13. Актуальная приёмка новой пары — после вечернего
> прогона 1.4.13 на всех четырёх колонках; этот файл её не заменяет.

# 2026-10-28: voice Previous / Restart, 1.4.10

- Owner accepted 1.4.9 on Lite 2 192.168.0.196: voice, Alice button and indication.
- Request: previous-track phrases send Previous twice; repeat/start-again sends
  it once, without setting repeat mode.
- Implemented in source/controls.go, covered by controls_previous_test.go:
  350 ms spacing, deduplication, cancellation, sender-change/disconnect/error
  guard before the second press. Existing voice/button/LED tests retained.
- Manifest limited to 32 phrases by YAM. Russian previous and restart phrases
  prioritized; some redundant pause/resume variants removed from the allowlist.
  Exact deployed list: workspace plugin.toml. Arbitrary paraphrases not promised.
- gofmt, go vet, go test -race, ARMv7 build, UPX and package validation PASS.
- Installed ONLY on .196. Runtime confirms 1.4.10 and fresh idle speech polls.
- Package SHA256:
  2ce028121ef23f72ea27eea316a20f4a0fd469e7dcd821ff89640ae65b2b0811
- No synthetic media commands or audible checks run; owner live acceptance of
  the new previous/restart behaviour remains pending. Other speakers untouched.
- Sender-dependent limitation: two Previous calls near song start can skip two
  items; one Previous is a restart only when the sender handles it that way.
- No firmware, rootfs, native ALSA, clocks, OTA or Quasar changes.
- Rollback package: C:/Temp/opencode/alice-button-20261028/station-airplay-1.4.9.yap
  SHA256 2f16f57b4dedc6dd2727be5ac44d822a388cb793b35cbffc56b46c09818daec3
  Install through install-station-airplay.ps1 with .196, preserving controls.
- Existing server backup: /home/rus/home-lab-agent/private/airplay-button-20261028/
- Next: owner tests previous and repeat from the middle of a song. No rollout
  to other speakers or publication without acceptance.
