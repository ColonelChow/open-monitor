package agent

import (
	"github.com/gin-gonic/gin"
	m "github.com/WeBankPartners/open-monitor/monitor-server/models"
	"github.com/WeBankPartners/open-monitor/monitor-server/services/prom"
	mid "github.com/WeBankPartners/open-monitor/monitor-server/middleware"
	"github.com/WeBankPartners/open-monitor/monitor-server/services/db"
	"strings"
	"fmt"
	"strconv"
	"time"
	"github.com/WeBankPartners/open-monitor/monitor-server/services/datasource"
)

const hostType  = "host"
const mysqlType  = "mysql"
const redisType = "redis"
const tomcatType = "tomcat"
const javaType = "java"
const otherType = "other"
var agentManagerUrl string

func RegisterAgent(c *gin.Context)  {
	var param m.RegisterParam
	if err := c.ShouldBindJSON(&param); err==nil {
		err = RegisterJob(param)
		if err != nil {
			mid.ReturnError(c, "Register failed", err)
			return
		}
		mid.ReturnSuccess(c, "Register successfully")
	}else{
		mid.ReturnValidateFail(c, fmt.Sprintf("Parameter validation failed %v", err))
	}
}

func RegisterJob(param m.RegisterParam) error {
	var err error
	//if param.Type != hostType && param.Type != mysqlType && param.Type != redisType && param.Type != tomcatType && param.Type != javaType && param.Type != otherType {
	//	return fmt.Errorf("Type " + param.Type + " is not supported yet")
	//}
	step := 10
	var strList []string
	var endpoint m.EndpointTable
	var tmpAgentIp,tmpAgentPort string
	if agentManagerUrl == "" {
		for _, v := range m.Config().Dependence {
			if v.Name == "agent_manager" {
				agentManagerUrl = v.Server
				break
			}
		}
	}
	if param.Type == hostType {
		err,strList = prom.GetEndpointData(param.ExporterIp, param.ExporterPort, []string{"node"}, []string{})
		if err != nil {
			mid.LogError("Get endpoint data failed ", err)
			return err
		}
		if len(strList) == 0 {
			return fmt.Errorf("Can't get anything from this address:port/metric ")
		}
		var hostname,sysname,release,exportVersion string
		for _,v := range strList {
			if strings.Contains(v, "node_uname_info{") {
				if strings.Contains(v, "nodename") {
					hostname = strings.Split(strings.Split(v, "nodename=\"")[1], "\"")[0]
				}
				if strings.Contains(v, "sysname") {
					sysname = strings.Split(strings.Split(v, "sysname=\"")[1], "\"")[0]
				}
				if strings.Contains(v, "release") {
					release = strings.Split(strings.Split(v, "release=\"")[1], "\"")[0]
				}
			}
			if strings.Contains(v, "node_exporter_build_info{") {
				exportVersion = strings.Split(strings.Split(v, ",version=\"")[1], "\"")[0]
			}
		}
		endpoint.Guid = fmt.Sprintf("%s_%s_%s", hostname, param.ExporterIp, hostType)
		endpoint.Name = hostname
		endpoint.Ip = param.ExporterIp
		endpoint.ExportType = hostType
		endpoint.Address = fmt.Sprintf("%s:%s", param.ExporterIp, param.ExporterPort)
		endpoint.OsType = sysname
		endpoint.Step = step
		endpoint.EndpointVersion = release
		endpoint.ExportVersion = exportVersion
	}else if param.Type == mysqlType{
		if param.Instance == "" {
			return fmt.Errorf("Mysql instance name can not be empty")
		}
		var binPath,address,configFile string
		if agentManagerUrl != "" {
			if param.User == "" || param.Password == "" {
				for _,v := range m.Config().Agent {
					if v.AgentType == mysqlType {
						param.User = v.User
						param.Password = v.Password
						binPath = v.AgentBin
						break
					}
				}
			}
			if param.User == "" || param.Password == "" {
				return fmt.Errorf("mysql monitor must have user and password to connect")
			}
			if binPath == "" {
				for _,v := range m.Config().Agent {
					if v.AgentType == mysqlType {
						binPath = v.AgentBin
						configFile = v.ConfigFile
						break
					}
				}
			}
			address,err = prom.DeployAgent(mysqlType,param.Instance,binPath,param.ExporterIp,param.ExporterPort,param.User,param.Password,agentManagerUrl,configFile)
			if err != nil {
				return err
			}
		}
		if address == "" {
			err, strList = prom.GetEndpointData(param.ExporterIp, param.ExporterPort, []string{"mysql", "mysqld"}, []string{})
		}else{
			if strings.Contains(address, ":") {
				tmpAddressList := strings.Split(address, ":")
				tmpAgentIp = tmpAddressList[0]
				tmpAgentPort = tmpAddressList[1]
				err, strList = prom.GetEndpointData(tmpAddressList[0], tmpAddressList[1], []string{"mysql", "mysqld"}, []string{})
			}else{
				mid.LogInfo(fmt.Sprintf("address : %s is bad", address))
				return fmt.Errorf("address : %s is bad", address)
			}
		}
		if err != nil {
			mid.LogError("curl endpoint data fail ", err)
			return err
		}
		if len(strList) == 0 {
			return fmt.Errorf("Can't get anything from this address:port/metric ")
		}
		if len(strList) <= 30 {
			return fmt.Errorf("Connect to instance fail,please check param ")
		}
		var mysqlVersion,exportVersion string
		for _,v := range strList {
			if strings.HasPrefix(v, "mysql_version_info{") {
				mysqlVersion = strings.Split(strings.Split(v, ",version=\"")[1], "\"")[0]
			}
			if strings.HasPrefix(v, "mysqld_exporter_build_info{") {
				exportVersion = strings.Split(strings.Split(v, ",version=\"")[1], "\"")[0]
			}
		}
		endpoint.Guid = fmt.Sprintf("%s_%s_%s", param.Instance, param.ExporterIp, mysqlType)
		endpoint.Name = param.Instance
		endpoint.Ip = param.ExporterIp
		endpoint.EndpointVersion = mysqlVersion
		endpoint.ExportType = mysqlType
		endpoint.ExportVersion = exportVersion
		endpoint.Step = step
		endpoint.Address = fmt.Sprintf("%s:%s", param.ExporterIp, param.ExporterPort)
		endpoint.AddressAgent = address
	}else if param.Type == redisType {
		if param.Instance == "" {
			return fmt.Errorf("Redis instance name can not be empty")
		}
		var binPath,address,configFile string
		if agentManagerUrl != "" {
			if binPath == "" {
				for _,v := range m.Config().Agent {
					if v.AgentType == redisType {
						binPath = v.AgentBin
						configFile = v.ConfigFile
						break
					}
				}
			}
			address,err = prom.DeployAgent(redisType,param.Instance,binPath,param.ExporterIp,param.ExporterPort,param.User,param.Password,agentManagerUrl,configFile)
			if err != nil {
				return err
			}
		}
		if address == "" {
			err, strList = prom.GetEndpointData(param.ExporterIp, param.ExporterPort, []string{"redis"}, []string{"redis_version", ",version"})
		}else{
			if strings.Contains(address, ":") {
				tmpAddressList := strings.Split(address, ":")
				tmpAgentIp = tmpAddressList[0]
				tmpAgentPort = tmpAddressList[1]
				err, strList = prom.GetEndpointData(tmpAddressList[0], tmpAddressList[1], []string{"redis"}, []string{"redis_version", ",version"})
			}else{
				mid.LogInfo(fmt.Sprintf("address : %s is bad", address))
				return fmt.Errorf("address : %s is bad", address)
			}
		}
		if err != nil {
			mid.LogError("curl endpoint data fail ", err)
			return err
		}
		if len(strList) == 0 {
			return fmt.Errorf("Can't get anything from this address:port/metric ")
		}
		if len(strList) <= 30 {
			return fmt.Errorf("Connect to instance fail,please check param ")
		}
		var redisVersion,exportVersion string
		for _,v := range strList {
			if strings.Contains(v, "redis_version") {
				mid.LogInfo(fmt.Sprintf("redis str list : %s", v))
				redisVersion = strings.Split(strings.Split(v, ",redis_version=\"")[1], "\"")[0]
			}
			if strings.Contains(v, ",version") {
				exportVersion = strings.Split(strings.Split(v, ",version=\"")[1], "\"")[0]
			}
		}
		endpoint.Guid = fmt.Sprintf("%s_%s_%s", param.Instance, param.ExporterIp, redisType)
		endpoint.Name = param.Instance
		endpoint.Ip = param.ExporterIp
		endpoint.EndpointVersion = redisVersion
		endpoint.ExportType = redisType
		endpoint.ExportVersion = exportVersion
		endpoint.Step = step
		endpoint.Address = fmt.Sprintf("%s:%s", param.ExporterIp, param.ExporterPort)
		endpoint.AddressAgent = address
	}else if param.Type == tomcatType || param.Type == javaType {
		if param.Instance == "" {
			return fmt.Errorf("Tomcat instance name can not be empty")
		}
		var binPath,address,configFile string
		if agentManagerUrl != "" {
			if param.User == "" || param.Password == "" {
				for _,v := range m.Config().Agent {
					if v.AgentType == tomcatType {
						param.User = v.User
						param.Password = v.Password
						binPath = v.AgentBin
						break
					}
				}
			}
			if param.User == "" || param.Password == "" {
				return fmt.Errorf("mysql monitor must have user and password to connect")
			}
			if binPath == "" {
				for _,v := range m.Config().Agent {
					if v.AgentType == tomcatType {
						binPath = v.AgentBin
						configFile = v.ConfigFile
						break
					}
				}
			}
			address,err = prom.DeployAgent(tomcatType,param.Instance,binPath,param.ExporterIp,param.ExporterPort,param.User,param.Password,agentManagerUrl,configFile)
			if err != nil {
				return err
			}
		}
		if address == "" {
			err, strList = prom.GetEndpointData(param.ExporterIp, param.ExporterPort, []string{"Catalina", "catalina", "jvm", "java", "Tomcat", "tomcat", "process", "com"}, []string{"version"})
		}else{
			if strings.Contains(address, ":") {
				tmpAddressList := strings.Split(address, ":")
				tmpAgentIp = tmpAddressList[0]
				tmpAgentPort = tmpAddressList[1]
				err, strList = prom.GetEndpointData(tmpAddressList[0], tmpAddressList[1], []string{"Catalina", "catalina", "jvm", "java"}, []string{"version"})
			}else{
				mid.LogInfo(fmt.Sprintf("address : %s is bad", address))
				return fmt.Errorf("address : %s is bad", address)
			}
		}
		if err != nil {
			mid.LogError("curl endpoint data fail ", err)
			return err
		}
		if len(strList) == 0 {
			return fmt.Errorf("Can't get anything from this address:port/metric ")
		}
		if len(strList) <= 60 {
			return fmt.Errorf("Connect to instance fail,please check param ")
		}
		var jvmVersion,exportVersion string
		for _,v := range strList {
			if strings.Contains(v, "jvm_info") {
				jvmVersion = strings.Split(strings.Split(v, "version=\"")[1], "\"")[0]
			}
			if strings.Contains(v, "jmx_exporter_build_info") {
				exportVersion = strings.Split(strings.Split(v, "version=\"")[1], "\"")[0]
			}
		}
		endpoint.Guid = fmt.Sprintf("%s_%s_%s", param.Instance, param.ExporterIp, param.Type)
		endpoint.Name = param.Instance
		endpoint.Ip = param.ExporterIp
		endpoint.EndpointVersion = jvmVersion
		endpoint.ExportType = param.Type
		endpoint.ExportVersion = exportVersion
		endpoint.Step = step
		endpoint.Address = fmt.Sprintf("%s:%s", param.ExporterIp, param.ExporterPort)
		endpoint.AddressAgent = address
	}else {
		if param.Instance == "" {
			return fmt.Errorf("Monitor endpoint name can not be empty")
		}
		endpoint.Guid = fmt.Sprintf("%s_%s_%s", param.Instance, param.ExporterIp, param.Type)
		endpoint.Name = param.Instance
		endpoint.Ip = param.ExporterIp
		endpoint.Address = fmt.Sprintf("%s:%s", param.ExporterIp, param.ExporterPort)
		endpoint.ExportType = param.Type
		endpoint.Step = step
	}
	err = db.UpdateEndpoint(&endpoint)
	if err != nil {
		mid.LogError( "Update endpoint failed ", err)
		return err
	}
	if param.Type == hostType || param.Type == mysqlType || param.Type == redisType || param.Type == tomcatType || param.Type == javaType {
		err = db.RegisterEndpointMetric(endpoint.Id, strList)
		if err != nil {
			mid.LogError("Update endpoint metric failed ", err)
			return err
		}
		if tmpAgentIp != "" && tmpAgentPort != "" {
			param.ExporterIp = tmpAgentIp
			param.ExporterPort = tmpAgentPort
		}
		err = prom.RegisteConsul(endpoint.Guid, param.ExporterIp, param.ExporterPort, []string{param.Type}, step, false)
		if err != nil {
			mid.LogError("Register consul failed ", err)
			return err
		}
	}
	return nil
}

func DeregisterAgent(c *gin.Context)  {
	guid := c.Query("guid")
	if guid == "" {
		mid.ReturnValidateFail(c, "Guid can not be empty")
		return
	}
	endpointObj := m.EndpointTable{Guid:guid}
	db.GetEndpoint(&endpointObj)
	if endpointObj.Id <= 0 {
		mid.ReturnError(c, fmt.Sprintf("Guid:%s can not find in table ", guid), nil)
		return
	}
	err := DeregisterJob(guid, endpointObj.Step)
	if err != nil {
		mid.ReturnError(c, fmt.Sprintf("Delete endpint %s failed", guid),err)
		return
	}
	mid.ReturnSuccess(c, fmt.Sprintf("Deregister %s successfully", guid))
}

func DeregisterJob(guid string,step int) error {
	err := db.DeleteEndpoint(guid)
	if err != nil {
		mid.LogError(fmt.Sprintf("Delete endpint %s failed", guid), err)
		return err
	}
	if m.Config().SdFile.Enable {
		prom.DeleteSdEndpoint(guid)
		err = prom.SyncSdConfigFile(step)
		if err != nil {
			mid.LogError("sync service discover file error: ", err)
			return err
		}
	}else {
		err = prom.DeregisteConsul(guid, false)
		if err != nil {
			mid.LogError(fmt.Sprintf("Deregister consul %s failed ", guid), err)
			return err
		}
	}
	db.UpdateAgentManagerTable(m.EndpointTable{Guid:guid}, "", "", "", "", false)
	return err
}

var TransGateWayAddress string

func CustomRegister(c *gin.Context)  {
	var param m.TransGatewayRequestDto
	if err:=c.ShouldBindJSON(&param); err==nil {
		if TransGateWayAddress == "" {
			query := m.QueryMonitorData{Start:time.Now().Unix()-60, End:time.Now().Unix(), Endpoint:[]string{"endpoint"}, Metric:[]string{"metric"}, PromQ:"up{job=\"transgateway\"}", Legend:"$custom_all"}
			sm := datasource.PrometheusData(&query)
			mid.LogInfo(fmt.Sprintf("sm length : %d ", len(sm)))
			if len(sm) > 0 {
				mid.LogInfo(fmt.Sprintf("sm0 -> %s  %s  %v", sm[0].Name, sm[0].Type, sm[0].Data))
				TransGateWayAddress = strings.Split(strings.Split(sm[0].Name, "instance=")[1], ",job")[0]
				mid.LogInfo(fmt.Sprintf("TransGateWayAddress : %s", TransGateWayAddress))
			}
		}
		var endpointObj m.EndpointTable
		endpointObj.Guid = fmt.Sprintf("%s_%s_custom", param.Name, param.HostIp)
		endpointObj.Address = TransGateWayAddress
		endpointObj.Name = param.Name
		endpointObj.Ip = param.HostIp
		endpointObj.ExportType = "custom"
		endpointObj.Step = 10
		err := db.UpdateEndpoint(&endpointObj)
		if err != nil {
			mid.ReturnError(c, fmt.Sprintf("Update endpoint %s_%s_custom fail", param.Name, param.HostIp), err)
		}else{
			mid.ReturnSuccess(c, "Success")
		}
	}else{
		mid.ReturnValidateFail(c, fmt.Sprintf("Parameter validate fail %v", err))
	}
}

func CustomMetricPush(c *gin.Context)  {
	var param m.TransGatewayMetricDto
	if err:=c.ShouldBindJSON(&param); err==nil {
		err = db.AddCustomMetric(param)
		if err != nil {
			mid.LogError("Add custom metric fail", err)
			mid.ReturnError(c, "Add custom metric fail", err)
		}else{
			mid.ReturnSuccess(c, "Success")
		}
	}else{
		mid.ReturnValidateFail(c, fmt.Sprintf("Parameter validate fail %v", err))
	}
}

func ReloadEndpointMetric(c *gin.Context)  {
	id,_ := strconv.Atoi(c.Query("id"))
	if id <= 0 {
		mid.ReturnValidateFail(c, "Param id validate fail")
		return
	}
	endpointObj := m.EndpointTable{Id:id}
	db.GetEndpoint(&endpointObj)
	var address string
	if endpointObj.Address == "" {
		if endpointObj.AddressAgent == "" {
			mid.ReturnError(c, fmt.Sprintf("Endpoint id %d have no address", id), nil)
			return
		}
		address = endpointObj.AddressAgent
	}else{
		address = endpointObj.Address
	}
	tmpExporterIp := strings.Split(address, ":")[0]
	tmpExporterPort := strings.Split(address, ":")[1]
	var strList []string
	if endpointObj.ExportType == hostType {
		_, strList = prom.GetEndpointData(tmpExporterIp, tmpExporterPort, []string{"node"}, []string{})
	}else if endpointObj.ExportType == mysqlType {
		_, strList = prom.GetEndpointData(tmpExporterIp, tmpExporterPort, []string{"mysql", "mysqld"}, []string{})
	}else if endpointObj.ExportType == redisType {
		_, strList = prom.GetEndpointData(tmpExporterIp, tmpExporterPort, []string{"redis"}, []string{"redis_version", ",version"})
	}else if endpointObj.ExportType == tomcatType {
		_, strList = prom.GetEndpointData(tmpExporterIp, tmpExporterPort, []string{"Catalina", "catalina", "jvm", "java"}, []string{"version"})
	}else{
		_, strList = prom.GetEndpointData(tmpExporterIp, tmpExporterPort, []string{}, []string{""})
	}
	err := db.RegisterEndpointMetric(id, strList)
	if err != nil {
		mid.ReturnError(c, "Update endpoint metric db fail", err)
	}else{
		mid.ReturnSuccess(c, "Success")
	}
}