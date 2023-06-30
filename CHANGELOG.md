# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

# Summary and Scope

These are changes to charts in support of:

## [1.19.0] - 2023-06-27

### Changed

- Refactored docker-compose file for runCT environment and replaced RTS with RIE.
- Updated CT tests to hms-test:5.1.0 image.
- Added non-disruptive, disruptive, and destructive Tavern CT tests for HMNFD.
- Made minor corrections and cleaned up the API swagger_v2 specification.

## [1.18.1] - 2023-01-12

### Changed

- Renamed the repo to hms-hmnfd, made some build changes to support M1 Mac (ARM).

## [1.18.0] - 2022-07-19

### Changed

- Updated CT tests to hms-test:3.2.0 image to pick up Helm test enhancements and CVE fixes.

## [1.17.0] - 2022-06-29

### Changed

- Switched HSM v1 to HSM v2.

## [1.16.0] - 2022-06-22

### Changed

- Updated CT tests to hms-test:3.1.0 image as part of Helm test coordination.

## [1.15.0] - 2022-05-12

### Changed

- Updated HMNFD to build using GitHub Actions instead of Jenkins.
- Pull images from artifactory.algol60.net instead of arti.dev.cray.com.
- Added a runCT.sh script that can run the CT tests in a docker-compose environment.
- Refactored runIntegration.sh and the disruptive Tavern integration tests.

## [1.14.0] - 2021-12-20

### Changed

- Documentation changes
- Fixed integration test issue with python

## [1.13.0] - 2021-12-20

### Added

- Now uses latest version of hms-msgbus, which now uses Confluent kafka interface.

## [1.12.0] - 2021-11-09

### Changed

- Added V2 API to support XName filtering in SCN subscriptions.

## [1.11.0] - 2021-11-04

### Added

- Fixed dependabot CVEs.

## [1.10.0] - 2021-10-27

### Added

- CASMHMS-5055 - Added HMNFD CT test RPM.

## [1.9.8] - 2021-09-21

### Changed

- Changed cray-service version to ~6.0.0

## [1.9.7] - 2021-09-08

### Changed

- Changed the docker image to run as the user nobody

## [1.9.6] - 2021-08-10

### Changed

- Added GitHub configuration files and fixed snyk warning.

## [1.9.5] - 2021-08-05

### Changed

- Added missing time stamps to Kafka telemetry SCNs.

## [1.9.4] - 2021-07-30

### Changed

- Added 'smart delays' between SCN send retries, and made the retry backoff and number of retries changeable on the fly.

## [1.9.3] - 2021-07-26

### Changed

- Phase 3 of Github migration.

## [1.9.2] - 2021-07-22

### Changed

- Add GH pipeline build support.

## [1.9.1] - 2021-07-12

### Security

- CASMHMS-4933 - Updated base container images for security updates.

## [1.9.0] - 2021-06-18

### Changed

- Bump minor version for CSM 1.2 release branch

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

- CASMHMS-2957 - Updated hms-hmnfd to use the latest trusted baseOS images.

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
