#!/bin/sh

# Must be provided by Jenkins credentials plugin:
# VAGRANTCLOUD_TOKEN

if [ -z "$VAGRANTCLOUD_TOKEN" ]
then
    echo "The environment variable VAGRANTCLOUD_TOKEN needs to be defined"
    exit 1
fi

dir="$(dirname "$0")"
. "${dir}/install-go.sh"

VERSION="$(grep 'skydive_release:' contrib/ansible/roles/skydive_common/defaults/main.yml | cut -f 2 -d ' ' | tr -d 'v')"

cd ${dir}/../../contrib/dev

vagrant plugin install vagrant-openstack
vagrant plugin install vagrant-reload

for provider in libvirt
do
  if [ "$provider" == "libvirt" ]; then
    sudo rmmod vboxnetadp vboxnetflt vboxpci vboxdrv
    sudo rmmod kvm_intel kvm
    sudo modprobe kvm_intel
    sudo systemctl restart libvirtd
  elif [ "$provider" == "virtualbox" ]; then
    sudo systemctl stop libvirtd
    sudo rmmod kvm_intel kvm
    sudo modprobe vboxdrv
    sudo modprobe vboxnetadp
    sudo modprobe vboxnetflt
    sudo modprobe vboxpci
  fi

  PREPARE_BOX=true vagrant up --provider=$provider
  if [ "$provider" == "libvirt" ]
  then
    sudo chmod a+r /var/lib/libvirt/images/dev_dev.img
  fi
  vagrant package --out skydive-dev-$provider.box
  vagrant destroy --force

  json=`curl "https://vagrantcloud.com/api/v1/box/skydive/skydive-dev/version/$VERSION/provider/$provider/upload?access_token=$VAGRANTCLOUD_TOKEN"`
  upload_path=`echo $json | jq .upload_path | cut -d '"' -f 2`
  curl -X PUT --upload-file skydive-dev-$provider.box $upload_path
done
