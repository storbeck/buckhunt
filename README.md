# Buck Hunt


A simple tool to check S3 buckets for public access and download their contents.

![duck_hunt_31623__90557](https://github.com/user-attachments/assets/9c953842-1c24-4425-9d3d-1f75f66fc33e)


## Features

- Check if an S3 bucket exists
- Test for public read/write permissions
- List bucket contents if readable
- Option to download entire bucket contents recursively

## Usage

Single bucket check with full output:
```bash
❯ ./buckhunt flaws.cloud                                                      

[+] Checking s3://flaws.cloud
[+] Public Read:  true
[+] Public Write: false

Files:
2017-03-14 03:00:38     2575 bytes      hint1.html
2017-03-03 04:05:17     1707 bytes      hint2.html
2017-03-03 04:05:11     1101 bytes      hint3.html
2024-02-22 02:32:41     2861 bytes      index.html
2018-07-10 16:47:16     15979 bytes     logo.png
2017-02-27 01:59:28     46 bytes        robots.txt
2017-02-27 01:59:30     1051 bytes      secret-dd02c7c.html

Download bucket contents? [y/N]: y
[+] Downloading bucket contents to directory: flaws.cloud
download: s3://flaws.cloud/hint2.html to flaws.cloud/hint2.html
download: s3://flaws.cloud/hint3.html to flaws.cloud/hint3.html
download: s3://flaws.cloud/hint1.html to flaws.cloud/hint1.html
download: s3://flaws.cloud/robots.txt to flaws.cloud/robots.txt
download: s3://flaws.cloud/index.html to flaws.cloud/index.html 
download: s3://flaws.cloud/logo.png to flaws.cloud/logo.png      
download: s3://flaws.cloud/secret-dd02c7c.html to flaws.cloud/secret-dd02c7c.html
[+] Download completed successfully
```

Bulk checking with quiet mode:
```bash
❯ cat domains.txt | ./buckhunt -q
flaws.cloud,true,false
level2-c8b217a33fcf1f839f6f1f73a00a9ae7.flaws.cloud,true,false
# Summary: 2 tested, 2 found (2 readable, 0 writable), 0 not found
```

## Flags

- `-q`: Quiet mode - outputs in CSV format (domain,read,write) for accessible buckets only, with a summary line

## Output Format

### Standard Mode
- Shows detailed information about each bucket
- Lists files with timestamps and sizes
- Prompts for downloading bucket contents

### Quiet Mode (-q)
- CSV format: `domain,read,write`
- Only shows accessible buckets (read or write permissions)
- Ends with a summary line starting with `#` showing:
  - Total buckets tested
  - Number of buckets found
  - Number with read access
  - Number with write access
  - Number not found

## Requirements

- Go 1.21 or later
- AWS CLI configured with credentials
- AWS credentials with S3 read permissions
