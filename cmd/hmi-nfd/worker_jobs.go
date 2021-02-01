// MIT License
//
// (C) Copyright [2019, 2021] Hewlett Packard Enterprise Development LP
//
// Permission is hereby granted, free of charge, to any person obtaining a
// copy of this software and associated documentation files (the "Software"),
// to deal in the Software without restriction, including without limitation
// the rights to use, copy, modify, merge, publish, distribute, sublicense,
// and/or sell copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
// THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR
// OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
// OTHER DEALINGS IN THE SOFTWARE.

package main

import (
	"errors"
	"log"
	"stash.us.cray.com/HMS/hms-base"
)

///////////////////////////////////////////////////////////////////////////////
// Job definitions
///////////////////////////////////////////////////////////////////////////////


const (
	JTYPE_INVALID  base.JobType = 0
	JTYPE_TEST     base.JobType = 1
	JTYPE_SCN_SEND base.JobType = 2
	JTYPE_MAX      base.JobType = 3
)

var JTypeString = map[base.JobType]string{
	JTYPE_INVALID:  "JTYPE_INVALID",
	JTYPE_TEST:     "JTYPE_TEST",
	JTYPE_SCN_SEND: "JTYPE_SCN_SEND",
	JTYPE_MAX:      "JTYPE_MAX",
}


///////////////////////////////////////////////////////////////////////////////
// Job: JTYPE_SCN_SEND
///////////////////////////////////////////////////////////////////////////////

type JobSCNSend struct {
    Status base.JobStatus
    Err error
    SCNData Scn
    Subscriber string
    Url string
}

/////////////////////////////////////////////////////////////////////////////
// Create a JTYPE_SCN_SEND job data structure.
//
// sd(in):         SCN data to send to a subscriber
// subscriber(in): XName of subscriber.
// url(in):        URL to send SCN to.
// Return:         Job data structure to be used by work Q.
/////////////////////////////////////////////////////////////////////////////

func NewJobSCNSend(sd Scn, subscriber string, url string) base.Job {
    j := new(JobSCNSend)
    j.Status = base.JSTAT_DEFAULT
    j.SCNData = sd
    j.Subscriber = subscriber
    j.Url = url
    return j
}

/////////////////////////////////////////////////////////////////////////////
// Log function for SCN send job.  Note that for now this is just a simple
// log call, but may be expanded in the future.
//
// format(in):  Printf-like format string.
// a(in):       Printf-like argument list.
// Return:      None.
/////////////////////////////////////////////////////////////////////////////

func (j *JobSCNSend) Log(format string, a ...interface{}) {
    log.Printf(format,a...)
}

/////////////////////////////////////////////////////////////////////////////
// Return current job type.
//
// Args: None
// Return: Job type.
/////////////////////////////////////////////////////////////////////////////

func (j *JobSCNSend) Type() base.JobType {
    return JTYPE_SCN_SEND
}

/////////////////////////////////////////////////////////////////////////////
// Run a job.  This is done by the worker pool when popping a job off of the
// work Q/chan.
//
// Args,Return: None.
/////////////////////////////////////////////////////////////////////////////

func (j *JobSCNSend) Run() {
    sendSCNToSubscriber(j.SCNData, j.Subscriber, j.Url)
}

/////////////////////////////////////////////////////////////////////////////
// Return the current job status and error info.
//
// Args: None
// Return: Current job status, and any error info (if any).
/////////////////////////////////////////////////////////////////////////////

func (j *JobSCNSend) GetStatus() (base.JobStatus,error) {
    if (j.Status == base.JSTAT_ERROR) {
        return j.Status,j.Err
    }
    return j.Status,nil
}

/////////////////////////////////////////////////////////////////////////////
// Set job status.
//
// newStatus(in): Status to set job to.
// err(in):       Error info to associate with the job.
// Return:        Previous job status; nil on success, error string on error.
/////////////////////////////////////////////////////////////////////////////

func (j *JobSCNSend) SetStatus(newStatus base.JobStatus, err error) (base.JobStatus,error) {
    if newStatus >= base.JSTAT_MAX {
        return j.Status, errors.New("Error: Invalid Status")
    } else {
        oldStatus := j.Status
        j.Status = newStatus
        j.Err = err
        return oldStatus, nil
    }
}

/////////////////////////////////////////////////////////////////////////////
// Cancel a job.  Note that this JobType does not support cancelling the 
// job while it is being processed
//
// Args:   None
// Return: Current job status before cancelling.
/////////////////////////////////////////////////////////////////////////////

func (j *JobSCNSend) Cancel() base.JobStatus {
	if (j.Status == base.JSTAT_QUEUED || j.Status == base.JSTAT_DEFAULT) {
		j.Status = base.JSTAT_CANCELLED
	}
	return j.Status
}

