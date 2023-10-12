# Monday logging CLI

Monday logging CLI is a tool to help create logging-related pulses on Monday
* Create pulse for a board and particular group, with a particular description and hours spent.

# Install

```sh
# Install or update by building from source
➜ go install github.com/denis-engcom/mlog/cmd/mlog@latest

# Install command during local development
➜ go build -o mlog cmd/mlog/*
# or
➜ go install ./cmd/mlog
```

# Setup

1. Run `mlog setup` which highlights the missing configuration data.

```sh
➜ mlog setup
User configuration path:   /Users/denis/Library/Application Support/mlog/config.toml
❌ Unable to parse file (missing or incorrectly formatted)
❌ Missing api_access_token
❌ Missing logging_user_id
(skipping boards configuration)
The user configuration has one or more validation errors.
Refer to github.com/denis-engcom/mlog - config.example.toml for how to configure the file properly.
```

2. Produce a config.toml in your user configuration path
    * Can copy config.example.toml from this repo as a template

```sh
➜ cp config.example.toml <config-path>/config.toml

# Open with a text editor, and modify
➜ open <config-path>/config.toml
````

3. Run `mlog update` to fetch required data for

```sh
➜ mlog update
GET https://denis-engcom.github.io/mlog/boards.toml (2374 bytes) - successful
Saved to /Users/denis/Library/Application Support/mlog/boards.toml
✅ Description: Board configuration covering months August 2023 to September 2023 (updated on 2023-10-11)
Update complete without errors.
```

4. Run `mlog setup` again, which validates that you're set up.

```sh
➜ mlog setup
User configuration path:   /Users/denis/Library/Application Support/mlog/config.toml
✅ File is valid
Boards configuration path: /Users/denis/Library/Application Support/mlog/boards.toml
✅ File is valid
✅ Description: Board configuration covering months August 2023 to September 2023 (updated on 2023-10-11)
Setup complete without errors.
```

# Usage

```sh
# Run without arguments or with `-h` or with `--help` to print usage
➜ mlog

# Get board items and summary to get an overview of logs against the monthly board.
➜ mlog get-board-items 2023-09
GROUP       HOURS  DESCRIPTION                   PULSE ID
-----       -----  -----------                   --------
Mon Sep 04  0.5    Daily Stand Up & Parking Lot  5678901234
Tue Sep 05  0.5    Daily Stand Up & Parking Lot  5678901235
Tue Sep 05  1      Demo/Code review meeting      5678901236
...

➜ mlog get-board-item-summary 2023-09
GROUP       TOTAL HOURS  PULSE COUNT
-----       -----------  -----------
Mon Sep 04  0.5          1
Tue Sep 05  1.5          2
...

# Create one log entry with info provided on the command line
# Day, log title, hours spent
# config.toml must be set up with credentials
# boards.toml must be up to date with the month's board information
# - As an example, mlog maps "2023-09-05" to board_id 1234567890 and group_id tue_sep_5
➜ mlog create-one 2023-09-05 "Pursued activities to get things done" 2.5
https://magicboard.monday.com/boards/1234567890/pulses/5678901237

# Quickly open a pulse in your browser for modification
➜ open `mlog pulse-link 5678901237`
```

## Admin - Prepare boards.toml content every month

```sh
# Gets board info in TOML form, see if it looks reasonable
➜ mlog admin-get-board-by-id <month-board-id>

# Using the desired month, store first lines
➜ mlog admin-get-board-by-id <month-board-id> | head -n 6 | sed s/yyyy-mm/<desired-month>/g >> docs/boards.toml

# Takes month's days, sorts numerically
➜ mlog admin-get-board-by-id <month-board-id> | tail -n 31 | sort -k 3.1,3.3 >> docs/boards.toml

# Update the description field in docs/boards.toml

# Test the config
➜ cp docs/boards.toml <personal-conf-path>/boards.toml

# Then, if it looks good, commit and push.
# The file will be made available for `mlog update` via github pages
```

## Future features to implement

Drive pulse creation from config (TOML or CSV)...

```sh
# All the information needed for log creation should be captured in the toml
# validate will use credentials to obtain board information, validate group (day of the month) values,
# and validate overall config format
➜ mlog create-all --dry-run april-2023.toml

# Send log entries to the Monday.com board
# For every log to be created, show information to user, and prompt for confirmation
➜ mlog create-all april-2023.toml
```

## Important links

[Monday API 2023-10 release notes](https://developer.monday.com/api-reference/docs/release-notes?lid=iur3fqsd7acz#2023-10)
