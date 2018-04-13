package core

import (
	"time"

	"github.com/ontology-oracle/log"
	"github.com/ontology-oracle/models"
	"github.com/ontology-oracle/runners"
	"github.com/robfig/cron"
)

func (app *OracleApplication) ExecuteCron(job *models.JobSpec) {
	c := cron.New()
	c.AddFunc(job.Scheduler.Params, func() {
		log.Infof("Cron job start, job ID is :%v", job.ID)
		jobRun := job.NewRun()
		jobRun = app.executeRun(jobRun)
		if jobRun.Status == models.RunStatusErrored {
			log.Errorf("Current job run execution error: %v", jobRun.Result.ErrorMessage)
		}
		if jobRun.Status == models.RunStatusCompleted {
			log.Infof("Finished current job run execution: %v, Job result is: %v", jobRun.ID, jobRun.Result)

			err := app.sendCronDataToContract(jobRun)
			if err != nil {
				log.Errorf("sendCronDataToContract error: %v", err.Error())
			} else {
				log.Infof("sendCronDataToContract success, Job ID is: %v", jobRun.JobID)
			}
		}
	})
	app.DoingJobs[job.ID] = new(interface{})
	c.Start()
}

func (app *OracleApplication) ExecuteRun(jobRun models.JobRun) {
	t, _ := time.Parse("2006-01-02 15:04:05", jobRun.Scheduler.Params)
	if t.After(time.Now()) {
		return
	}
	app.DoingJobs[jobRun.JobID] = new(interface{})
	jobRun = app.executeRun(jobRun)
	if jobRun.Status == models.RunStatusErrored {
		log.Errorf("Current job run execution error: %v", jobRun.Result.ErrorMessage)
	}
	if jobRun.Status == models.RunStatusCompleted {
		log.Infof("Finished current job run execution: %v, Job result is: %v", jobRun.ID, jobRun.Result)

		err := app.sendDataToContract(jobRun)
		if err != nil {
			log.Errorf("sendDataToContract error: %v", err.Error())
		} else {
			log.Infof("sendDataToContract success, Job ID is: %v", jobRun.JobID)
		}
	}
	delete(app.DoingJobs, jobRun.JobID)
}

func (app *OracleApplication) executeRun(jobRun models.JobRun) models.JobRun {
	jobRun.Status = models.RunStatusInProgress

	log.Infof("Starting job run: %v, JobID is: %v", jobRun.ID, jobRun.JobID)
	unfinished := jobRun.UnfinishedTaskRuns()
	offset := len(jobRun.TaskRuns) - len(unfinished)
	latestRun := unfinished[0]

	for i, taskRun := range unfinished {

		log.Debugf("Starting task run: %v", taskRun.ID)
		latestRun = markCompleted(startTask(taskRun, latestRun.Result))
		jobRun.TaskRuns[i+offset] = latestRun
		if latestRun.Result.Status == models.RunStatusErrored {
			break
		}
		log.Debugf("Finish task run: %v", taskRun.ID)

	}

	jobRun = jobRun.ApplyResult(latestRun.Result)
	return jobRun
}

func markCompleted(tr models.TaskRun) models.TaskRun {
	if tr.Status.Runnable() {
		return tr.MarkCompleted()
	}
	return tr
}

func startTask(taskRun models.TaskRun, input models.RunResult) models.TaskRun {

	taskRun.Status = models.RunStatusInProgress
	runner, err := runners.For(taskRun.Task)

	if err != nil {
		taskRun.Status = models.RunStatusErrored
		taskRun.Result.ErrorMessage = err.Error()
		return taskRun
	}

	return taskRun.ApplyResult(runner.Perform(input))
}
