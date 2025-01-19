# Buck Hunt

A tool to check S3 buckets for public access and AWS credentials access, featuring an interactive TUI mode for bulk checking.

Interactve mode with a file input
![demo](https://github.com/user-attachments/assets/fb85d4b6-2026-41f5-8c1a-0d2588ba42eb)

Interactive mode with a stream input
![demo](https://github.com/user-attachments/assets/3841c0e3-fb37-4da4-9878-e8bbad807a46)

Quiet mode
![demo](https://github.com/user-attachments/assets/0f50abc1-13d9-4bbb-8a48-94be7ba8be4f)

## Features

- Interactive TUI (Terminal User Interface) for bulk checking
- Check if S3 buckets exist and their accessibility
- Test access via AWS credentials
- Parallel processing with configurable workers
- CSV output mode for automation
- Smart handling of wildcard domains
- Real-time progress tracking
- Detailed statistics and summary

## Usage

### Interactive TUI Mode (Default for Multiple Domains)
```bash
❯ cat domains.txt | ./buckhunt
⚡ Buck Hunter
Press 'q' to quit

Found:
level2-c8b217a33fcf1f839f6f1f73a00a9ae7.flaws.cloud AWS
flaws.cloud READ

Testing: example-bucket.s3.amazonaws.com
Summary: 45 tested, 2 found (1 readable, 0 writable, 1 aws), 43 not found
```

### Single Bucket Check
```bash
❯ ./buckhunt level2-c8b217a33fcf1f839f6f1f73a00a9ae7.flaws.cloud
Found bucket level2-c8b217a33fcf1f839f6f1f73a00a9ae7.flaws.cloud:
  Read:  false
  Write: false
  AWS:   true
```

### Quiet Mode (CSV Output)
```bash
❯ cat domains.txt | ./buckhunt -q
flaws.cloud,true,false,false
level2-c8b217a33fcf1f839f6f1f73a00a9ae7.flaws.cloud,false,false,true
# Summary: 2 tested, 2 found (1 readable, 0 writable, 1 aws), 0 not found
```

## Flags

- `-q`: Quiet mode - outputs in CSV format (domain,public_read,public_write,aws_read)
- `-w`: Number of concurrent workers (default: 20, max: 100)

## Output Formats

### Interactive TUI Mode
- Real-time progress display
- Live results as buckets are found
- Color-coded access indicators:
  - READ: Public read access
  - WRITE: Public write access
  - AWS: Accessible via AWS credentials
- Detailed summary on completion

### Quiet Mode (-q)
- CSV format: `domain,public_read,public_write,aws_read`
- One line per accessible bucket
- Summary line with statistics

## Performance

- Default 20 concurrent workers
- Configurable up to 100 workers with `-w` flag
- Smart queuing system for efficient processing
- Graceful handling of large input sets
- Skips wildcard domains automatically

## Requirements

- Go 1.21 or later
- AWS CLI configured with credentials (for AWS access checks)
