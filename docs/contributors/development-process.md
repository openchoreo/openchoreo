This document describes the development process used by the OpenChoreo project.

# Overview

The OpenChoreo project uses an agile development process. We go through one week long development 
sprints to design and implement the roadmap items. 

Each Monday maintainers and contributors are expected to do the retrospective and planning and then continue with the week's assigned tasks.

# Issue Tracking
We use two GitHub boards to track issues.
- [OpenChoreo Release Management](https://github.com/orgs/openchoreo/projects/5) - Tracks higher level 
feature requirements, improvements and bugs. Features will be represented with type/epic issues. 
- [OpenChoreo Development](https://github.com/orgs/openchoreo/projects/7) - Development teams track 
tasks that will deliver new product features, bug fixes, improvements to the product. Only type/task issues will be added to this board.

# Issue Triage Process

This document outlines the issue triage process for contributors. The goal in issue triaging is to go through newly logged issues and incorporate them into the development process appropriately. Issue triaging will help to keep the open issue count and backlog healthy.

## Definitions

### Priority Labels

Priority labels are to be assigned by leads based on the severity and complexity of the issues. As an example, less severe but also less complex issues will have a high priority. Priority is also tied to the ETA as described below. Priority can change between triaging sessions and is affected by other external reasons.

* Priority/Highest  
  Issue should be dealt with immediately and added to the current sprint  
* Priority/High  
  Issue should be dealt with immediately and added to the next sprint  
* Priority/Normal  
  Issue should be dealt with within the current release  
* Priority/Low  
  It is ok to fix the issue in the next release

## Goals

* Keep an approved limited backlog for the current release   
  * Teams will have consistent flow of work  
  * Maintainers and contributors have the option to review effective backlog and make dynamic decisions  
* Keep the unattended issue count minimal  
  * Frequent triages would help maintainers and contributors to give attention to all the raised issues  
* Keep a healthy roadmap for next release  
  * PM will have a collection of vetted requests for the next release cycle planning

## Issue triage frequency

The issue triage process should happen at least once a week. If the issue count is not manageable, maintainers and contributors should plan to do several triage sessions to bring the issue count down.

## Issue triage process

Issue triaging should be done using Github issues. Maintainers and contributors should first filter issues based on the below criteria.

* Belongs to your Area label  
* Belongs to New Feature, Improvement, Bug issue types
* Not in 'OpenChoreo Release Management' project board

Then for each issue type follow the below sections

### Feature triage process

Type/Feature represents feature requests opened by the community. These requests should be first taken through the approval process according to this (./../proposals/README.md) document. If approval is not granted then the issue should be closed off with the reason. If approval is granted:
* Open a Type/Epic issue representing the new feature
* Add relevant details to the epic
* Add the epic to the [OpenChoreo Release Management](https://github.com/orgs/openchoreo/projects/5) project board and assign the milestone(v1.0.0, v1.1.0), area and priority.
* Close off the New Feature issue pointing to the epic

### Improvement triage process
* Initiate a discussion within the issue to decide the validity of the requirement
* If the improvement request is valid then assign the relevant milestone, area and priority.
* Add the improvement to the [OpenChoreo Release Management](https://github.com/orgs/openchoreo/projects/5) project board

### Bug Triage Process
* Initiate a discussion within the issue to decide the validity of the bug
* If the bug is valid then assign the relevant milestone, area and priority.
* 
* Add the bug to the [OpenChoreo Release Management](https://github.com/orgs/openchoreo/projects/5) project board



# Inside a Sprint

## Milestone Retrospective Meeting
> *Attendees*:
> - Development Team/Contributor
>
> *Time and Date*: First Monday of the sprint

Each team gets together and retrospects the last sprint

#### Key Tasks
- Review issues in PR Sent / In Review status and try to close them off
- Review issues still in progress and include a comment explaining why we could not finish them as planned and change the sprint to the current sprint
- Send updates to Discord with screencast when applicable for finished tasks

## Milestone Planning Meeting
> *Attendees*:
> - Development Team/Contributor
>
> *Time and Date*: First Monday of the sprint

Each team/contributor gets together and plans for the current sprint by picking tasks from the [OpenChoreo Development](https://github.com/orgs/openchoreo/projects/7) board backlog and moving them to the current iteration.

#### Key Tasks
- Pick tasks from backlog for the current sprint
- If there aren't enough tasks in the backlog go through the Release Management Board and pick epics/improvements/bugs based on priority
- For Epic and Improvement issues separately groom the issue and create subtasks accordingly and add subtasks to the OpenChoreo Development board backlog
- Add issues to the current iteration
- Add Area labels if not present
- Assign issues to contributors accordingly
