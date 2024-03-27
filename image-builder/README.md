# RKE2 CAPI image builder

## Description

This directory contains the scripts and configuration files to build the images used by the RKE2 CAPI provider. It relies on the [packer](https://www.packer.io/) tool to build the images.

We are using bash scripts to provision images with required dependencies, scripts for each platform are located in the `scripts` directory.

## AWS

### Requirements

- Your AWS account must have the following permissions: https://developer.hashicorp.com/packer/plugins/builders/amazon#iam-task-or-instance-role

- You must a default VPC in your AWS account. If you don't have one, you can create one using the following command:

```bash
aws ec2 create-default-vpc
```

### Steps

For building the AWS AMIs, you can run the following command, this command will build a private openSUSE AMI:

```bash
make build-aws-opensuse-leap-155 RKE2_VERSION=${RKE2_VERSION}
```
or 

```bash
make help
```
and it will show you the available options.
