package gitcamli

import (
	"context"
	"errors"
	"strings"

	"camlistore.org/pkg/blob"
	"camlistore.org/pkg/schema"
	"camlistore.org/pkg/schema/nodeattr"
	"camlistore.org/pkg/search"
	"github.com/dkolbly/git"
)

var ErrBadStore = errors.New("bad store")
var ErrNotFound = errors.New("no ref with name")

// SetNamed sets an attribute on a permanode that represents
// the named reference.  Named references are pointers to
// commits that have names, such as branches (heads) and tags.
// We model such a thing as a permanode with the following attributes:
//
//    camliContent  - the commit's blobref
//    gitRepository - the name of the repository
//    tag           - the type of reference (either "heads" or "tags")
//    title         - the name of the reference
//
// we cache the link to the permanode
func (be *Client) SetNamed(ref *git.NamedRef) (bool, error) {
	perma, err := be.getPermanode(ref)
	if err != nil {
		return false, err
	}

	content, err := be.getRefFromPerma(perma)
	if err == nil && content != nil && *content == ref.Ptr {
		log.Debug("already set")
		return false, nil
	}
	log.Debug("updating %s -> %s", perma, &ref.Ptr)

	if err != nil {
		//return err
		log.Warning("Rats: %s", err)
	}

	assoc := schema.NewSetAttributeClaim(
		perma,
		nodeattr.CamliContent,
		"sha1-"+ref.Ptr.String())

	p2, err := be.camli.UploadAndSignBlob(assoc)
	if err != nil {
		log.Error("Failed to record content claim: %s", err)
		return false, err
	}

	log.Info("%s claim: %s", perma, p2.BlobRef)
	return true, nil
}

var noRef = blob.Ref{}

func (be *Client) getPermanode(ref *git.NamedRef) (blob.Ref, error) {

	/*
		repo := be.repoName
		key := fmt.Sprintf("$named[%s] %s %s", repo, ref.RefType, ref.Name)

		// first, see if there is an existing permanode
		data, err := be.cache.Get([]byte(key), nil)
		if err == nil {
			bref, ok := blob.Parse(string(data))
			if ok {
				return bref, nil
			}
			log.Warning("invalid permanode link")
			// pretend like it wasn't there; fall through to recreate it
			err = leveldb.ErrNotFound
		}

		if err != leveldb.ErrNotFound {
			return noRef, err
		}
	*/

	// we didn't have one in cache... see if it exists on the server
	bref, err := be.findPermanode(ref)

	if err == ErrNotFound {
		// it wasn't on the server either... create it
		bref, err = be.makePermanode(ref)
		if err != nil {
			return noRef, err
		}
		log.Debug("Created new permanode in store: %s", bref)
	} else if err != nil {
		return noRef, err
	} else {
		log.Debug("Found permanode in store: %s", bref)
	}
	/*
		// cache it
		err = be.cache.Put([]byte(key), []byte(bref.String()), nil)
		if err != nil {
			return noRef, err
		}*/

	return bref, nil
}

// these are pure camlistore operations:

func (be *Client) findPermanode(ref *git.NamedRef) (blob.Ref, error) {

	repo := be.repoName
	attrEq := func(k, v string) *search.Constraint {
		return &search.Constraint{
			Permanode: &search.PermanodeConstraint{
				Attr:  k,
				Value: v,
			},
		}
	}

	hasTag := attrEq("tag", ref.RefType.String())
	repoIsNamed := attrEq("gitRepository", repo)
	titleIs := attrEq("title", ref.Name)

	and := func(a, b *search.Constraint) *search.Constraint {
		return &search.Constraint{
			Logical: &search.LogicalConstraint{
				Op: "and",
				A:  a,
				B:  b,
			},
		}
	}

	q := &search.SearchQuery{
		Constraint: and(hasTag, and(repoIsNamed, titleIs)),
	}

	result, err := be.camli.Query(q)
	if err != nil {
		return noRef, err
	}
	if len(result.Blobs) == 0 {
		return noRef, ErrNotFound
	}
	return result.Blobs[0].Blob, nil
}

func (be *Client) makePermanode(ref *git.NamedRef) (blob.Ref, error) {
	repo := be.repoName
	put, err := be.camli.UploadNewPermanode()
	if err != nil {
		log.Error("Failed to create permanode: %s", err)
		return noRef, err
	}
	perma := put.BlobRef
	log.Info("Created permanode for [%s] %s %s: %s",
		repo,
		ref.Name,
		&ref.Ptr,
		perma)

	//----------------------------------------
	assoc := schema.NewSetAttributeClaim(
		perma,
		"gitRepository",
		repo)
	p2, err := be.camli.UploadAndSignBlob(assoc)
	if err != nil {
		log.Error("Failed to record content claim: %s", err)
		return noRef, err
	}
	log.Info("%s repo claim: %s", perma, p2.BlobRef)

	//----------------------------------------
	assoc = schema.NewSetAttributeClaim(
		perma,
		"tag",
		ref.RefType.String())
	p2, err = be.camli.UploadAndSignBlob(assoc)
	if err != nil {
		log.Error("Failed to record content claim: %s", err)
		return noRef, err
	}

	log.Info("%s tag claim: %s", perma, p2.BlobRef)

	//----------------------------------------
	assoc = schema.NewSetAttributeClaim(
		perma,
		"title",
		ref.Name)
	p2, err = be.camli.UploadAndSignBlob(assoc)
	if err != nil {
		log.Error("Failed to record content claim: %s", err)
		return noRef, err
	}

	log.Info("%s title claim: %s", perma, p2.BlobRef)

	return perma, nil
}

func (be *Client) getRefFromPerma(perma blob.Ref) (*git.Ptr, error) {

	q := &search.DescribeRequest{
		BlobRefs: []blob.Ref{
			perma,
		},
	}

	desc, err := be.camli.Describe(context.Background(), q)
	if err != nil {
		log.Error("Could not describe %s: %s", perma, err)
		return nil, err
	}
	md := desc.Meta.Get(perma)
	if md == nil {
		return nil, ErrNotFound
	}
	if len(md.Permanode.Attr[nodeattr.CamliContent]) < 1 {
		return nil, nil
	}

	cc := md.Permanode.Attr[nodeattr.CamliContent][0]
	bref, ok := blob.Parse(cc)
	if !ok {
		log.Error("camliContent %q is not a blobref", cc)
		return nil, ErrBadStore
	}
	if !strings.HasPrefix(bref.String(), "sha1-") {
		log.Error("bref %q is not sha1", bref)
		return nil, ErrBadStore
	}

	p, ok := git.ParsePtr(bref.String()[5:])
	if ok {
		return &p, nil
	} else {
		log.Error("bref %q is not a git ref", bref.String()[5:])
		return nil, ErrBadStore
	}
}

func (cc *Client) GetNamed(t git.RefType, name string) *git.NamedRef {
	tmp := git.NamedRef{
		RefType: t,
		Name:    name,
	}
	bref, err := cc.findPermanode(&tmp)
	if err != nil {
		log.Error("find permanode for %s %s: %s", t, name, err)
		return nil
	}
	ptr, err := cc.getRefFromPerma(bref)
	if err != nil {
		log.Error("get ref from %s failed: %s", bref, err)
		return nil
	}

	log.Info("%s %s ==> %s", t, name, ptr)
	tmp.Ptr = *ptr
	return &tmp
}
