# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html) with 0.x semantics: minor versions may break the API.

## [Unreleased]

## [0.5.4] - 2026-09-02

### Changed

- rio v0.18.1.

## [0.5.3] - 2026-09-02

### Changed

- rio v0.18.0.

## [0.5.2] - 2026-09-02

### Changed

- rio v0.17.0.

## [0.5.1] - 2026-09-02

### Added

- `CONTRIBUTING.md`, `CHANGELOG.md`, `llms.txt`, and compile-only examples for `Open` and `New`.

### Changed

- README restructured, with the `loc`/`time_zone` interaction documented. No API change.

## [0.5.0] - 2026-09-02

### Added

- Duplicate-key errors 1022 (`ER_DUP_KEY`), 1169 (`ER_DUP_UNIQUE`), and 1586 (`ER_DUP_ENTRY_WITH_KEY_NAME`) translate to `rio.ErrDuplicateKey`; foreign-key errors 1216 (`ER_NO_REFERENCED_ROW`) and 1217 (`ER_ROW_IS_REFERENCED`) translate to `rio.ErrForeignKeyViolated`.

### Changed

- `Open` and `New` enable rio's prepared-statement cache by default, so a parameterized statement costs one round trip after its first use; `rio.WithoutStmtCache()` opts out.
- rio v0.16.0.

## [0.4.1] - 2026-08-31

### Changed

- rio v0.13.0.

## [0.4.0] - 2026-08-30

### Changed

- `Open` rejects `clientFoundRows=true`: rio's write semantics rely on changed-rows counting.
- rio v0.11.0.

## [0.3.1] - 2026-08-20

### Changed

- Go 1.27 and MySQL 9 support through dependency updates.

## [0.3.0] - 2026-08-09

### Changed

- rio v0.10.0.

## [0.2.3] - 2026-07-11

### Changed

- rio v0.9.0; release automation.

## [0.2.2] - 2026-07-10

### Changed

- rio v0.7.0.

## [0.2.1] - 2026-07-10

### Changed

- rio v0.6.0.

## [0.2.0] - 2026-07-10

### Added

- `Open` rejects `sql_mode` values that change placeholder lexing (`NO_BACKSLASH_ESCAPES`, `ANSI_QUOTES`, and the combination modes implying them).

### Changed

- rio v0.5.0.

## [0.1.0] - 2026-07-09

### Added

- Initial release: `Open` and `New`, `parseTime` enforcement, duplicate-key and foreign-key error translation.

[Unreleased]: https://github.com/go-rio/mysql/compare/v0.5.4...HEAD
[0.5.4]: https://github.com/go-rio/mysql/compare/v0.5.3...v0.5.4
[0.5.3]: https://github.com/go-rio/mysql/compare/v0.5.2...v0.5.3
[0.5.2]: https://github.com/go-rio/mysql/compare/v0.5.1...v0.5.2
[0.5.1]: https://github.com/go-rio/mysql/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/go-rio/mysql/compare/v0.4.1...v0.5.0
[0.4.1]: https://github.com/go-rio/mysql/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/go-rio/mysql/compare/v0.3.1...v0.4.0
[0.3.1]: https://github.com/go-rio/mysql/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/go-rio/mysql/compare/v0.2.3...v0.3.0
[0.2.3]: https://github.com/go-rio/mysql/compare/v0.2.2...v0.2.3
[0.2.2]: https://github.com/go-rio/mysql/compare/v0.2.1...v0.2.2
[0.2.1]: https://github.com/go-rio/mysql/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/go-rio/mysql/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/go-rio/mysql/releases/tag/v0.1.0
