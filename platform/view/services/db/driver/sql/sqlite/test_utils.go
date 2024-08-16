package sqlite

//type TestDriver struct {
//	Name    string
//	ConnStr string
//}
//
//func (t *TestDriver) NewTransactionalVersioned(dataSourceName string, config driver.Config) (driver.TransactionalVersionedPersistence, error) {
//	return initPersistence(NewPersistence, t.ConnStr, t.Name, 50)
//}
//
//func (t *TestDriver) NewVersioned(dataSourceName string, config driver.Config) (driver.VersionedPersistence, error) {
//	return initPersistence(NewPersistence, t.ConnStr, t.Name, 50)
//}
//
//func (t *TestDriver) NewUnversioned(dataSourceName string, config driver.Config) (driver.UnversionedPersistence, error) {
//	return initPersistence(NewUnversioned, t.ConnStr, t.Name, 50)
//}
//
//func (t *TestDriver) NewTransactionalUnversioned(dataSourceName string, config driver.Config) (driver.TransactionalUnversionedPersistence, error) {
//	p, err := initPersistence(NewPersistence, t.ConnStr, t.Name, 50)
//	if err != nil {
//		return nil, err
//	}
//	return &unversioned.Transactional{TransactionalVersioned: p}, nil
//}
//
