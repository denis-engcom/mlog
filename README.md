# Monday logging CLI

Monday logging CLI is a tool to help create pulses in a particular fashion on Monday

## Install

```sh
# Install by building from source
➜ go install github.com/denis-engcom/mlog/cmd/mlog@latest

# Run without arguments or with `-h` or with `--help` to print usage
➜ mlog

# Install command during local development
➜ go build -o mlog cmd/mlog/*
# or
➜ go install ./cmd/mlog
```

## Example usage

```sh
# Get board information to inform the user on creating logs against this board.
# Board ID
➜ mlog get-board 1234567890

# Create one log entry with info provided on the command line
# Board ID, column ID, log title, hours spent
# Minimal config.toml must be set up with target user and credentials
➜ mlog create-one 1234567890 mon_apr_1 "Pursued activities to get things done" 2.5
```

## Future features to implement

```sh
# All the information needed for log creation should be captured in the toml
# validate will use credentials to obtain board information, validate group (day of the month) values,
# and validate overall config format
➜ mlog validate-config april-2023.toml

# Send log entries to the Monday.com board
# For every log to be created, show information to user, and prompt for confirmation
➜ mlog create-all april-2023.toml
```
