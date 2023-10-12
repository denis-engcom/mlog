# Monday logging CLI

Monday logging CLI is a tool to help create logging-related pulses on Monday
* Create pulse for a board and particular group, with a particular description and hours spent.

## Install

```sh
# Install by building from source
➜ go install github.com/denis-engcom/mlog/cmd/mlog@latest

# Install command during local development
➜ go build -o mlog cmd/mlog/*
# or
➜ go install ./cmd/mlog
```

## Example usage

First, produce a config.toml in your working directory
* Can copy config.example.toml from this repo as a template

```sh
# Run without arguments or with `-h` or with `--help` to print usage
➜ mlog

# Get board information to inform the user on creating logs against this board.
# Board ID
# Pipe the output trough a program like `jq` if wanting pretty printed or filtered
➜ mlog get-board-by-id 1234567890
{"ID":"1234567890","Name":"Apr 2023 Completed Work","Columns":[{"ID":"name","Title":"Name"},...],"Groups":[{"ID":"mon_apr_1","Title":"Mon Apr 1"},...]}
```

Populate `boards.toml` with board information
```toml
[months.2023-04]
board_id = 1234567890
[months.2023-04.days]
"-01": "mon_apr_1"
# ...
```

```sh
# Create one log entry with info provided on the command line
# Day, log title, hours spent
# Minimal config.toml must be set up with target user and credentials
# mlog maps "2023-04-01" to board_id 1234567890 and group_id mon_apr_1
➜ mlog create-one 2023-04-01 "Pursued activities to get things done" 2.5
https://magicboard.monday.com/boards/1234567890/pulses/3216540987
```

## Admin - Prepare boards.toml content every month

```sh
# Gets board info in TOML form, see if it looks reasonable
➜ mlog get-board-by-id <month-board-id>

# Using the desired month, store first lines
➜ mlog get-board-by-id <month-board-id> | head -n 6 | sed s/yyyy-mm/<desired-month>/g >> docs/boards.toml

# Takes month's days, sorts numerically
➜ mlog get-board-by-id <month-board-id> | tail -n 31 | sort -k 3.1,3.3 >> docs/boards.toml

# Update the description field in docs/boards.toml

# Test the config
➜ cp docs/boards.toml <personal-conf-path>/boards.toml

# Then, if it looks good, commit and push.
# The file will be made avaiable for `mlog update` via github pages
```

## Future features to implement

Drive pulse creation from config (TOML or CSV)...

```sh
# All the information needed for log creation should be captured in the toml
# validate will use credentials to obtain board information, validate group (day of the month) values,
# and validate overall config format
➜ mlog validate-config april-2023.toml

# Send log entries to the Monday.com board
# For every log to be created, show information to user, and prompt for confirmation
➜ mlog create-all april-2023.toml
```

## Important links

[Monday API 2023-10 release notes](https://developer.monday.com/api-reference/docs/release-notes?lid=iur3fqsd7acz#2023-10)
