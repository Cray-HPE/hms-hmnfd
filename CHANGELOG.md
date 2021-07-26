# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

# Summary and Scope

These are changes to charts in support of:

## [1.8.3] - 2021-07-26

### Changed 

- Github migration phase 3.

## [1.8.2] - 2021-07-22

### Changed 

- Added pipeline support for building in GH 

## [1.8.1] - 2021-07-01

### Security

- CASMHMS-4898 - Updated base container images for security updates.

## [1.8.0] - 2021-06-18

### Changed

- Bump minor version for CSM 1.1 release branch

## [1.7.4] - 2021-05-04

### Changed

- Updated docker-compose files to pull images from Artifactory instead of DTR.

## [1.7.3] - 2021-04-16

### Changed

- Updated Dockerfiles to pull base images from Artifactory instead of DTR.

## [1.7.2] - 2021-03-03

### Changed

- Increased the pod resource limits.

## [1.7.1] - 2021-02-03

### Added

- Update Copyright/license and re-vendor go packages.

## [1.7.0] - 2021-02-01

### Added

- Update Copyright/license and re-vendor go packages.

## [1.6.2] - 2021-01-26

### Added

- Added time stamps to each SCN.

## [1.6.1] - 2021-01-20

### Added

- Added User-Agent headers to all outbound HTTP requests.

## [1.6.0] - 2021-01-14

### Changed

- Updated license file.

## [1.5.2] - 2020-10-20

- CASMHMS-4105 - Updated base Golang Alpine image to resolve libcrypto vulnerability.

## [1.5.1] - 2020-10-01

- Upgraded to base service chart 2.0.1

## [1.5.0] - 2020-09-15

- moving to Helm v1/Loftsman v1
- the newest 2.x cray-service base chart
  - upgraded to support Helm v3
  - modified containers/init containers, volume, and persistent volume claim value definitions to be objects instead of arrays
- the newest 0.2.x cray-jobs base chart
  - upgraded to support Helm v3

## [1.4.2] - 2020-08-12

- CASMHMS-2957 - Updated hms-hmi-nfd to use the latest trusted baseOS images.

## [1.4.1] - 2020-06-30

- CASMHMS-3628 - Updated HMNFD CT smoke test with new API test cases.

## [1.4.0] - 2020-06-26

- Bumped the base chart to 1.11.1 for ETCD improvements. Updated istio pod annotation to exclude ETCD. 

## [1.3.5] - 2020-06-12

- Bumped base chart to 1.8 for ETCD improvements.

## [1.3.4] - 2020-06-05

- Now caches and coalesces similar inbound SCNs to reduce the number of subscriber SCN transactions.  Also drops dead subscribers when disconnected.

## [1.3.3] - 2020-06-01

- Now supports online install/upgrade/downgrade.

## [1.3.2] - 2020-05-27

- Bumped ETCD resource limits.  Changed base service chart rev from 1.3.x to 1.5.x.

## [1.3.1] - 2020-05-26

- Improved queueing of inbound SCNs.

## [1.3.0] - 2020-05-15

- Now retries opening connection to ETCD on startup forever, eliminating the need for wait-for-etcd job.  Also removed the use of the HMS common repo.

## [1.2.0] - 2020-4-03

- Now runs 3 copies of hmnfd.  Trimmed some debug print spam.

## [1.1.6] - 2020-3-26

- Bumped base Helm chart level to use an ETCD config change.

## [1.1.5] - 2020-3-2

- Update cray-service dependency to 1.2.0 to pull in etcd volume fix

## [1.1.4] - 2020-1-24

### Changed

- CASMHMS-2636: Added liveness, readiness, and health endpoints to the service.

## [1.1.3] - 2019-11-24

### Changed

- Changed the URL sent to HSM from the API-GW version to the in-mesh URL, e.g. http://cray-hmnfd/hmi/v1

## [1.1.2] - 2019-10-24

### Changed

- Now uses re-usable HTTP transport and client rather than creating one for each outbound request.

## [1.1.1] - 2019-08-02

### Fixed

- Initial subscription to SCNs from HSM now correctly retries until it succeeds.

## [1.1.0] - 2019-05-14

### Changed

- Moved files around to match new standard.

### Removed

- `hmi-service` from this new repo.

### Fixed

- Made `runUnitTest.sh` work with new directory layout.

## [1.0.0] - 2019-05-13

### Added

- This is the initial release. It contains everything that was in `hms-services` at the time with the major exception of being `go mod` based now.

### Changed

### Deprecated

### Removed

### Fixed

### Security
