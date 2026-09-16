// Package validate is Grove's leaf validation engine: the tag rules
// (`required,min,max,len,gte,lte,email,uuid,url,oneof` + RegisterRule)
// with zero dependencies. pipes wraps it with HTTP error mapping;
// core uses it for declarative DTO handlers. Import this package when
// you need validation without HTTP semantics.
package validate
