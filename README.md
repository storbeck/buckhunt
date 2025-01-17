# Buck Hunt


A simple tool to check S3 buckets for public access and download their contents.

![duck_hunt_31623__90557](https://github.com/user-attachments/assets/9c953842-1c24-4425-9d3d-1f75f66fc33e)


## Features

- Check if an S3 bucket exists
- Test for public read/write permissions via HTTP
- Check access via AWS credentials
- List bucket contents (both public and AWS-authenticated)
- Option to download entire bucket contents recursively
- Parallel processing for bulk checking

## Usage

Single bucket check with full output:
```bash
❯ ./buckhunt level2-c8b217a33fcf1f839f6f1f73a00a9ae7.flaws.cloud

[+] Checking s3://level2-c8b217a33fcf1f839f6f1f73a00a9ae7.flaws.cloud
[+] Public Read:  false
[+] Public Write: false
[+] AWS Read:     true

Files (via AWS):
2017-02-26 21:02:15     80751 bytes     everyone.png
2017-03-02 22:47:17     1433 bytes      hint1.html
2017-02-26 21:04:39     1035 bytes      hint2.html
2017-02-26 21:02:14     2786 bytes      index.html
2017-02-26 21:02:14     26 bytes        robots.txt
2017-02-26 21:02:15     1051 bytes      secret-e4443fc.html

Download bucket contents? [y/N]: y
[+] Downloading bucket contents to directory: level2-c8b217a33fcf1f839f6f1f73a00a9ae7.flaws.cloud
...
```

Bulk checking with quiet mode:
```bash
❯ cat domains.txt | ./buckhunt -q -w 20
flaws.cloud,true,false,false
level2-c8b217a33fcf1f839f6f1f73a00a9ae7.flaws.cloud,false,false,true
# Summary: 2 tested, 2 found (1 readable, 0 writable, 1 aws), 0 not found
```

## Flags

- `-q`: Quiet mode - outputs in CSV format (domain,public_read,public_write,aws_read) for accessible buckets only
- `-w`: Number of concurrent workers for parallel processing (default: 10, max: 100)

## Output Format

### Standard Mode
- Shows detailed information about each bucket
- Displays public and AWS access status
- Lists files with timestamps and sizes (from HTTP or AWS depending on access)
- Prompts for downloading bucket contents

### Quiet Mode (-q)
- CSV format: `domain,public_read,public_write,aws_read`
- Only shows accessible buckets (public or AWS access)
- Ends with a summary line starting with `#` showing:
  - Total buckets tested
  - Number of buckets found
  - Number with public read access
  - Number with public write access
  - Number accessible via AWS
  - Number not found

### Performance
- Uses Go routines for parallel processing
- Default 10 concurrent workers, configurable with `-w`
- Automatically scales workers based on input size
- Example: `cat domains.txt | ./buckhunt -q -w 20`

## Requirements

- Go 1.21 or later
- AWS CLI configured with credentials
- AWS credentials with S3 read permissions
