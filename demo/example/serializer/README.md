## Serializer 序列化器


Kubernetes目前支持json、yaml、protobuf三种数据格式的序列化和反序列化，有必要抽象序列化和反序列化的统一接口
#### k8s序列化器，包括：序列化与反序列化器。
#### 序列化器：对象 -> 字符串
#### 反序列化器：字符串 -> 对象

![](https://github.com/googs1025/k8s-client-go-api-demo/blob/main/image/serializer.jpg?raw=true)

```bigquery
// Serializer是用于序列化和反序列化API对象的核心接口
type Serializer interface {
    // Serializer继承了编码器和解码器，编码器就是用来序列化API对象的，序列化的过程称之为编码；反之，反序列化的过程称之为解码。
	Encoder
	Decoder
}
 
// 序列化的过程称之为编码，实现编码的对象称之为编码器(Encoder)
type Encoder interface {
    // Encode()将对象写入流。可以将Encode()看做为json(yaml).Marshal()，只是输出变为io.Writer。
	Encode(obj Object, w io.Writer) error
 
    // Identifier()返回编码器的标识符，当且仅当两个不同的编码器编码同一个对象的输出是相同的，那么这两个编码器的标识符也应该是相同的。
    // 也就是说，编码器都有一个标识符，两个编码器的标识符可能是相同的，判断标准是编码任意API对象时输出都是相同的。
    // 标识符有什么用？标识符目标是与CacheableObject.CacheEncode()方法一起使用，CacheableObject又是什么东东？后面有介绍。
	Identifier() Identifier
}
 
// 标识符就是字符串，可以简单的理解为标签的字符串形式，后面会看到如何生成标识符。
type Identifier string
 
// 反序列化的过程称之为解码，实现解码的对象称之为解码器(Decoder)
type Decoder interface {
    // Decode()尝试使用Schema中注册的类型或者提供的默认的GVK反序列化API对象。
    // 如果'into'非空将被用作目标类型，接口实现可能会选择使用它而不是重新构造一个对象。
    // 但是不能保证输出到'into'指向的对象，因为返回的对象不保证匹配'into'。
    // 如果提供了默认GVK，将应用默认GVK反序列化，如果未提供默认GVK或仅提供部分，则使用'into'的类型补全。
	Decode(data []byte, defaults *schema.GroupVersionKind, into Object) (Object, *schema.GroupVersionKind, error)
}

```
