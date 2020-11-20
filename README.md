Kontainer Engine LKE Driver
===============================

This is the Kontainer-Engine Linode Driver. It is used in conjunction with the [ui-cluster-driver-lke.](https://github.com/linode/ui-cluster-driver-lke)

### Packaging and distributing


## Building

`make`

Will output driver binaries into the `dist` directory, these can be imported 
directly into Rancher and used as cluster drivers.  They must be distributed 
via URLs that your Rancher instance can establish a connection to and download 
the driver binaries.  For example, this driver is distributed via a GitHub 
release and can be downloaded from one of those URLs directly.


## Running

Go to the `Cluster Drivers` management screen in Rancher and click 
`Add Cluster Driver`. Enter the URL of your driver, a UI URL (see the UI 
[ui-cluster-driver-lke](https://github.com/linode/ui-cluster-driver-lke) for details), and a 
checksum (optional), and click `Create`. Rancher will automatically download 
and install your driver. It will then become available to use on the 
`Add Cluster` screen.

## License
Copyright (c) 2018 [Rancher Labs, Inc.](http://rancher.com)

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

[http://www.apache.org/licenses/LICENSE-2.0](http://www.apache.org/licenses/LICENSE-2.0)

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
