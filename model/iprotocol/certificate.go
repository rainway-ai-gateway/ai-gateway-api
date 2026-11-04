// Copyright (c) 2021 The BFE Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package iprotocol

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"time"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ibasic"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ioperlog"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/itxn"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iversion_control"
)

const certificateExpiredDateLayout = "2006-01-02 15:04:05"

type Certificate struct {
	CertName    string
	Description string
	IsDefault   bool

	CertFilePath string
	KeyFilePath  string
	ExpiredDate  string

	Products []*ibasic.Product
}

type CertificateFilter struct {
	CertName  *string
	IsDefault *bool
}

type CertificateParam struct {
	CertName    *string
	Description *string
	IsDefault   *bool

	CertFilePath    *string
	CertFileContent *string
	KeyFileContent  *string
	KeyFilePath     *string
	ExpiredDate     *string
}

type CertificateStorager interface {
	FetchCertificates(context.Context, *CertificateFilter) ([]*Certificate, error)
	DeleteCertificate(context.Context, *Certificate) error
	CreateCertificate(context.Context, *CertificateParam) error
	UpdateCertificate(context.Context, *Certificate, *CertificateParam) error
}

type CertificateManager struct {
	storager          CertificateStorager
	txn               itxn.TxnStorager
	extraFileStorager ibasic.ExtraFileStorager

	versionControlManager *iversion_control.VersionControlManager
	operationLogManager   ioperlog.OperationLogRecorder
}

func NewCertificateManager(txn itxn.TxnStorager, storager CertificateStorager,
	versionControlManager *iversion_control.VersionControlManager,
	extraFileStorager ibasic.ExtraFileStorager) *CertificateManager {
	return &CertificateManager{
		txn:               txn,
		storager:          storager,
		extraFileStorager: extraFileStorager,

		versionControlManager: versionControlManager,
	}
}

// SetOperationLogManager injects the operation log recorder.
func (pm *CertificateManager) SetOperationLogManager(manager ioperlog.OperationLogRecorder) {
	pm.operationLogManager = manager
}

func (pm *CertificateManager) FetchCertificates(ctx context.Context, param *CertificateFilter) (list []*Certificate, err error) {
	err = pm.txn.AtomExecute(ctx, func(ctx context.Context) error {
		list, err = pm.storager.FetchCertificates(ctx, param)
		return err
	})

	return
}

func (pm *CertificateManager) DeleteCertificate(ctx context.Context, certificate *Certificate) (err error) {
	if certificate.IsDefault {
		err = xerror.WrapModelErrorWithMsg("Cant Delete Default Certificate")
		pm.recordCertificateOperation(ctx, string(ioperlog.ActionDelete), certificate.CertName, certificate.CertName, "", certificateToMap(certificate), nil, err)
		return
	}

	if len(certificate.Products) > 0 {
		err = xerror.WrapConflictErrorWithMsg("Cant Delete Certificate Be Refer By Product")
		pm.recordCertificateOperation(ctx, string(ioperlog.ActionDelete), certificate.CertName, certificate.CertName, "", certificateToMap(certificate), nil, err)
		return
	}

	err = pm.txn.AtomExecute(ctx, func(ctx context.Context) error {
		if err := pm.extraFileStorager.DeleteExtraFile(ctx, &ibasic.ExtraFileFilter{
			Names: []string{certificate.CertFilePath, certificate.KeyFilePath},
		}); err != nil {
			return err
		}

		return pm.storager.DeleteCertificate(ctx, certificate)
	})
	if err != nil {
		pm.recordCertificateOperation(ctx, string(ioperlog.ActionDelete), certificate.CertName, certificate.CertName, "", certificateToMap(certificate), nil, err)
		return
	}

	pm.recordCertificateOperation(ctx, string(ioperlog.ActionDelete), certificate.CertName, certificate.CertName, "", certificateToMap(certificate), nil, nil)
	return
}

func validateCertPair(certFileContent string, keyFileContent string) error {
	checkCertFileInfo := func(certPEMBlock []byte) error {
		var certDERBlock *pem.Block
		for {
			certDERBlock, certPEMBlock = pem.Decode(certPEMBlock)
			if certDERBlock == nil {
				break
			}

			if certDERBlock.Type != "CERTIFICATE" {
				return xerror.WrapParamErrorWithMsg("Certificate File Format Must Be PEM")
			}
		}
		return nil
	}

	if err := checkCertFileInfo([]byte(certFileContent)); err != nil {
		return err
	}

	checkKeyFileInfo := func(keyPEMBlock []byte) error {
		var keyDERBlock *pem.Block
		keyDERBlock, _ = pem.Decode(keyPEMBlock)
		if keyDERBlock == nil {
			return xerror.WrapParamErrorWithMsg("Certificate Private Key File Format Must Be PEM")
		}
		if keyDERBlock.Type != "PRIVATE KEY" && !strings.HasSuffix(keyDERBlock.Type, " PRIVATE KEY") {
			return xerror.WrapParamErrorWithMsg("Certificate Private Key File Format Must Be PEM")
		}
		return nil
	}

	if err := checkKeyFileInfo([]byte(keyFileContent)); err != nil {
		return err
	}

	checkCertKeyPair := func(certPEMBlock, keyPEMBlock []byte) error {
		// crypto/tls is the same implementation the data plane now uses to
		// load certificates, so an accepted pair is also loadable by bfe.
		_, err := tls.X509KeyPair(certPEMBlock, keyPEMBlock)
		return err
	}

	if err := checkCertKeyPair([]byte(certFileContent), []byte(keyFileContent)); err != nil {
		return err
	}

	return nil
}

func parseExpiredDate(certFileContent string) (string, error) {
	block, _ := pem.Decode([]byte(certFileContent))
	if block == nil {
		return "", xerror.WrapParamErrorWithMsg("Certificate File Format Must Be PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", xerror.WrapParamErrorWithMsg("Certificate File Format Must Be X509")
	}

	// Reference time package to avoid "imported and not used".
	_ = time.UTC
	return cert.NotAfter.UTC().Format(certificateExpiredDateLayout), nil
}

func (pm *CertificateManager) CreateCertificate(ctx context.Context, param *CertificateParam) (err error) {
	certName := ""
	if param.CertName != nil {
		certName = *param.CertName
	}

	if err = validateCertPair(*param.CertFileContent, *param.KeyFileContent); err != nil {
		pm.recordCertificateOperation(ctx, string(ioperlog.ActionCreate), certName, certName, "", nil, certificateParamToMap(param), err)
		return err
	}

	expiredDate, err := parseExpiredDate(*param.CertFileContent)
	if err != nil {
		pm.recordCertificateOperation(ctx, string(ioperlog.ActionCreate), certName, certName, "", nil, certificateParamToMap(param), err)
		return err
	}
	param.ExpiredDate = &expiredDate

	err = pm.txn.AtomExecute(ctx, func(ctx context.Context) error {
		list, err := pm.storager.FetchCertificates(ctx, nil)
		if err != nil {
			return err
		}

		var defaultCertificate *Certificate
		// validate name unique
		for _, one := range list {
			if one.CertName == *param.CertName {
				return xerror.WrapRecordExisted("Certification")
			}

			if one.IsDefault {
				defaultCertificate = one
			}
		}

		if defaultCertificate == nil && (param.IsDefault == nil || !*param.IsDefault) {
			return xerror.WrapModelErrorWithMsg("Must Has Default Certificate")
		}
		// check/update default
		if isDefault := param.IsDefault; isDefault != nil && *isDefault && defaultCertificate != nil {
			if err = pm.storager.UpdateCertificate(ctx, defaultCertificate, &CertificateParam{
				IsDefault: lib.PBool(false),
			}); err != nil {
				return err
			}
		}

		certFileName := *param.CertName + ".crt"
		keyFileName := *param.CertName + ".key"
		param.CertFilePath = lib.PString(ibasic.ExtraFilePath(tlsConfDir, ibasic.BuildinProduct, certFileName))
		param.KeyFilePath = lib.PString(ibasic.ExtraFilePath(tlsConfDir, ibasic.BuildinProduct, keyFileName))

		if err := pm.extraFileStorager.CreateExtraFile(ctx, ibasic.BuildinProduct, &ibasic.ExtraFileParam{
			Name:    param.CertFilePath,
			Content: []byte(*param.CertFileContent),
		}, &ibasic.ExtraFileParam{
			Name:    param.KeyFilePath,
			Content: []byte(*param.KeyFileContent),
		}); err != nil {
			return err
		}

		return pm.storager.CreateCertificate(ctx, param)
	})
	if err != nil {
		pm.recordCertificateOperation(ctx, string(ioperlog.ActionCreate), certName, certName, "", nil, certificateParamToMap(param), err)
		return
	}

	pm.recordCertificateOperation(ctx, string(ioperlog.ActionCreate), certName, certName, "", nil, certificateParamToMap(param), nil)

	return
}

func (pm *CertificateManager) UpdateAsDefaultCertificate(ctx context.Context, cert *Certificate) (err error) {
	if cert.IsDefault {
		return nil
	}

	err = pm.txn.AtomExecute(ctx, func(ctx context.Context) error {

		list, err := pm.storager.FetchCertificates(ctx, &CertificateFilter{
			IsDefault: lib.PBool(true),
		})
		if err != nil {
			return err
		}
		if len(list) != 0 {
			if err = pm.storager.UpdateCertificate(ctx, list[0], &CertificateParam{
				IsDefault: lib.PBool(false),
			}); err != nil {
				return err
			}
		}

		return pm.storager.UpdateCertificate(ctx, cert, &CertificateParam{
			IsDefault: lib.PBool(true),
		})
	})
	if err != nil {
		pm.recordCertificateOperation(ctx, string(ioperlog.ActionUpdate), cert.CertName, cert.CertName, "", certificateToMap(cert), map[string]interface{}{
			"cert_name":  cert.CertName,
			"is_default": true,
		}, err)
		return
	}

	pm.recordCertificateOperation(ctx, string(ioperlog.ActionUpdate), cert.CertName, cert.CertName, "", certificateToMap(cert), map[string]interface{}{
		"cert_name":  cert.CertName,
		"is_default": true,
	}, nil)
	return
}
