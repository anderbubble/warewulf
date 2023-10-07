#!/usr/bin/bash

dnf install -y epel-release
dnf install -y mock

RELEASE=$(rpm -q --qf "%{RELEASE}\n" --specfile warewulf.spec | cut -d. -f1-2)
echo "RELEASE=${RELEASE}" >> $GITHUB_ENV
VERSION=$(rpm -q --qf "%{VERSION}\n" --specfile warewulf.spec)

mock -r rocky+epel-8-x86_64 --rebuild --spec=warewulf.spec --sources=.
mv /var/lib/mock/rocky+epel-8-x86_64/result/warewulf-${VERSION}-${RELEASE}.el8.x86_64.rpm .
mv /var/lib/mock/rocky+epel-8-x86_64/result/warewulf-${VERSION}-${RELEASE}.el8.src.rpm .

mock -r centos+epel-7-x86_64 --rebuild --spec=warewulf.spec --sources=.
mv /var/lib/mock/centos+epel-7-x86_64/result/warewulf-${VERSION}-${RELEASE}.el7.x86_64.rpm .
mv /var/lib/mock/centos+epel-7-x86_64/result/warewulf-${VERSION}-${RELEASE}.el7.src.rpm .

mock -r opensuse-leap-15.3-x86_64 --rebuild --spec=warewulf.spec --sources=.
mv /var/lib/mock/opensuse-leap-15.3-x86_64/result/warewulf-${VERSION}-${RELEASE}.suse.lp153.x86_64.rpm .
mv /var/lib/mock/opensuse-leap-15.3-x86_64/result/warewulf-${VERSION}-${RELEASE}.suse.lp153.src.rpm .
