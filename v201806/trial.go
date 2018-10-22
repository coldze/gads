package v201806

type TrialService struct {
	Auth
}

func NewTrialService(auth *Auth) *TrialService {
	return &TrialService{Auth: *auth}
}
