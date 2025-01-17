# Buckhunt

A simple tool to check S3 buckets for public access and download their contents.

## Features

- Check if an S3 bucket exists
- Test for public read/write permissions
- List bucket contents if readable
- Option to download entire bucket contents recursively

## Usage

```bash
go run main.go <domain>
```

Example:
```bash
go run main.go flaws.cloud
```

## Requirements

- Go 1.21 or later
- AWS CLI configured with credentials
- AWS credentials with S3 read permissions

## Notes

- Downloaded bucket contents will be saved in a directory named after the bucket
- The program uses AWS CLI's `s3 sync` command for downloading
- All bucket contents are ignored by git (see .gitignore)
